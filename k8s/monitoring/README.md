# Support Desk metrics in Minikube

Run commands from the repository root. Requires Docker, Minikube, kubectl, and
Helm 4.1.0 or newer. Reuse the existing profile and PVCs. Install Helm using the
[official instructions](https://helm.sh/docs/intro/install/).

The deployment follows steps 1–2 of the
[VictoriaMetrics single-node guide](https://docs.victoriametrics.com/guides/k8s-monitoring-via-vm-single/).
The single-node server performs discovery, scraping, and storage. Scrape status
is available on its port 8428; there is no separate vmagent process or port 8429.

## Restart an existing installation

After starting a stopped profile, wait for the existing workloads to recover
before opening port-forwards or recording the demonstration:

```sh
minikube start --profile=minikube
kubectl --context=minikube wait --for=condition=Ready node/minikube --timeout=300s
kubectl --context=minikube -n support-desk rollout status deployment/postgres --timeout=300s
kubectl --context=minikube -n support-desk rollout status deployment/support-app --timeout=300s
kubectl --context=minikube -n monitoring rollout status statefulset/victoria-metrics --timeout=300s
kubectl --context=minikube -n support-desk get pods
kubectl --context=minikube -n monitoring get pods,pvc
```

A profile restart can temporarily show `CreateContainerConfigError` with
`failed to sync configmap cache` while kubelet reconnects to the API and Docker
runtime. Application containers can also restart while PostgreSQL recovers.
During verification these states cleared without changing the ConfigMap or
Secret. Wait for PostgreSQL `2/2`, each application Pod `1/1`, and VictoriaMetrics
`1/1`, then check that all scrape targets are `up`.

If a readiness wait fails, inspect the actual error before continuing:

```sh
kubectl --context=minikube -n support-desk get events --field-selector type=Warning --sort-by=.lastTimestamp
kubectl --context=minikube -n support-desk describe pod <failing-pod-name>
```

## Build and deploy

Use the host Docker daemon. If necessary, undo an earlier `docker-env` setup
with `eval "$(minikube docker-env -u)"` first.

```sh
minikube start --profile=minikube
kubectl --context=minikube get nodes,storageclass
docker build --build-arg VERSION=slo-v1 -t support-desk:slo-v1 .
docker build -f scripts/Dockerfile.prober -t support-prober:slo-v1 .
docker pull victoriametrics/victoria-metrics:v1.154.0
docker pull quay.io/prometheuscommunity/postgres-exporter:v0.20.1
docker pull postgres:18.0-alpine3.22@sha256:48c8ad3a7284b82be4482a52076d47d879fd6fb084a1cbfccbd551f9331b0e40
minikube image load support-desk:slo-v1 support-prober:slo-v1 victoriametrics/victoria-metrics:v1.154.0 quay.io/prometheuscommunity/postgres-exporter:v0.20.1 postgres:18.0-alpine3.22@sha256:48c8ad3a7284b82be4482a52076d47d879fd6fb084a1cbfccbd551f9331b0e40
./k8s/deploy.sh
./k8s/monitoring/deploy.sh
```

The application script waits for PostgreSQL, migrations, and seeding before
starting the server. It declares one application replica. PostgreSQL uses its
existing PVC and a sidecar exporter connecting to localhost with the existing
demonstration credentials referenced through the ConfigMap and Secret.

The monitoring script installs release `vm-single`, chart `0.49.0`, into
`monitoring`, using [values.yaml](values.yaml). `HELM` can point to an alternative
Helm executable; `KUBE_CONTEXT` defaults to `minikube`. The fixed resource name is
`victoria-metrics`. It uses a 2 GiB PVC, seven-day retention, and an internal
headless ClusterIP Service. The script restarts the StatefulSet after an upgrade
to load changed scrape configuration; the chart does not checksum this ConfigMap.

The chart creates a ServiceAccount in `monitoring`. The Role and RoleBinding in
[discovery-rbac.yaml](discovery-rbac.yaml) grant only get/list/watch on Pods in
`support-desk`. Discovery reads the Pod-template `prometheus.io/*` annotations,
selects only the application and exporter ports, and attaches namespace, Pod,
and container labels. Neither application nor PostgreSQL Service publishes the
metrics port. Do not add the same targets through a second scraper.

[rendered.yaml](rendered.yaml) contains the chart resources rendered with the
checked-in values, without live Secrets. Regenerate after editing values:

```sh
helm template vm-single victoria-metrics-single \
  --repo https://victoriametrics.github.io/helm-charts/ --version 0.49.0 \
  --namespace monitoring --values k8s/monitoring/values.yaml \
  > k8s/monitoring/rendered.yaml
```

Use Helm and the deployment script to manage these resources. Do not recursively
apply `k8s/`: the application Jobs require ordering and the monitoring directory
also contains Helm values and a rendered reference copy.

## Generate activity and inspect metrics

Keep each port-forward in a separate terminal:

```sh
kubectl --context=minikube -n support-desk port-forward deployment/support-app 18080:8080 19090:9090
kubectl --context=minikube -n monitoring port-forward service/victoria-metrics 8428:8428
```

The Pod forward provides temporary localhost access to the internal metrics
listener. The ordinary application Service deliberately has no metrics port.
Reconnect forwards when their selected Pod is replaced.

- Application: http://localhost:18080
- Application metrics: http://localhost:19090/metrics
- VictoriaMetrics UI: http://localhost:8428/vmui/
- Scrape targets: http://localhost:8428/targets
- Discovery details: http://localhost:8428/service-discovery

Wait at least two scrape intervals (30 seconds) before starting traffic so rates
and increases have a baseline. Then run:

```sh
python3 scripts/metrics-traffic.py --rounds 4
curl -fsS http://localhost:8428/api/v1/targets
curl -fsS http://localhost:8428/api/v1/label/__name__/values
curl -fsSG http://localhost:8428/api/v1/query --data-urlencode 'query=up'
curl -fsSG http://localhost:8428/api/v1/query --data-urlencode 'query=pg_up{job="postgres"}'
curl -fsSG http://localhost:8428/api/v1/query --data-urlencode 'query=sum by (action) (support_ticket_actions_total{job="support-desk"})'
curl -fsS -H 'Accept: application/openmetrics-text; version=1.0.0' http://localhost:19090/metrics
```

The script creates/reuses separate demonstration user and operator accounts,
logs both in, then creates one ticket and two explicit comments per round. It
exercises every workflow action and three successful resolutions per ticket,
plus rejected credentials, permission checks, and duplicate actions. It prints
expected event increments and HTTP statuses without credentials or cookies.
Each invocation leaves its tickets and conversation messages in the database.
The default invitation matches the local demonstration Secret. For a different
installation, set `OPERATOR_INVITE_CODE` or pass `--operator-invite`.

Metric definitions, label bounds, resolution-time semantics, and MetricsQL
examples are documented in the [report](../../docs/reports/report_week_4_metrics.md).
`up` describes successful scraping; `pg_up` verifies that the exporter can query
PostgreSQL. A reachable exporter alone does not establish database health.

## Verify discovery and persistence

```sh
kubectl --context=minikube -n support-desk scale deployment/support-app --replicas=3
kubectl --context=minikube -n support-desk rollout status deployment/support-app --timeout=180s
```

After discovery and scraping, `/api/v1/targets` should show three healthy
`support-desk` targets and one `postgres` target. A Service/Deployment port-forward
selects one Pod; use the target list to verify that all replicas are scraped.
Replace one application Pod and confirm its new address appears automatically.

Restart VictoriaMetrics with:

```sh
kubectl --context=minikube -n monitoring rollout restart statefulset/victoria-metrics
kubectl --context=minikube -n monitoring rollout status statefulset/victoria-metrics --timeout=180s
```

Reconnect its forward and query an earlier interval to verify historical samples
remain. Application counters reset with the process, while stored samples remain
on the VictoriaMetrics PVC. Use `sum(rate(...))` or `sum(increase(...))` across
replicas, calculating each series' rate/increase before summing.

## Checks and diagnostics

```sh
kubectl --context=minikube -n monitoring get pods,services,pvc
kubectl --context=minikube -n monitoring logs statefulset/victoria-metrics --tail=50
kubectl --context=minikube -n support-desk logs deployment/postgres -c postgres-exporter --tail=50
kubectl --context=minikube auth can-i list pods -n support-desk --as=system:serviceaccount:monitoring:victoria-metrics
```

Backend validation uses `go test -race ./...` and `go vet ./...` from `backend/`.
Set `TEST_DATABASE_URL` to a disposable PostgreSQL database to enable integration
tests; their harness truncates application tables. The tests cover business
increments, rollback, concurrent actions, repeated resolutions, OpenMetrics
negotiation, port separation, configuration, and listener cleanup.

The SLO lab also deploys `support-prober:slo-v1` and creates its credential Secret once.
See [the SLO guide](../slo/README.md) for the Grafana dashboard and degradation demonstration.
