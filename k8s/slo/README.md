# Support Desk SLI / SLO / SLA lab

## Build and deploy

```sh
minikube start --profile=minikube
# Use the host Docker daemon, not minikube docker-env.
docker build --build-arg VERSION=slo-v1 -t support-desk:slo-v1 .
docker build -f scripts/Dockerfile.prober -t support-prober:slo-v1 .
minikube image load support-desk:slo-v1 support-prober:slo-v1
./k8s/slo/deploy.sh
```

The previous PostgreSQL, exporter, VictoriaMetrics, Vector and Grafana images
must also be pullable; see the monitoring and logging guides. Helm must be on
PATH, or set `HELM=/path/to/helm`.

The application deploy script creates a random prober password Secret once.
Keep this Secret on redeploy, because the dedicated accounts persist in the DB.
The prober uses cookies, checks real 201/200 results and cleans up transient
tickets/comments. One fixture ticket and its user/operator accounts remain.
Each 10-second round independently probes ticket creation, comment creation and
operator reassignment. A failure never skips the other business probes.

VictoriaMetrics scrapes the app and prober every 15 seconds. Three metrics per
operation (attempts, successes and last result), a latency histogram and a
last-round timestamp are available at the prober's internal port 9115. Its
readiness endpoint deliberately reports process health, not business health, so
Kubernetes continues exposing failed probe results.

## Open Grafana and verify

Run port-forwards in separate terminals:

```sh
kubectl --context=minikube -n logging port-forward service/grafana 13000:3000
kubectl --context=minikube -n monitoring port-forward service/victoria-metrics 8428:8428
```

Open http://localhost:13000/d/support-slo with the existing Grafana admin account.
To copy its password privately:

```sh
kubectl --context=minikube -n logging get secret grafana-admin -o jsonpath='{.data.password}' | base64 -d | xclip -selection clipboard
```

The datasource is `VictoriaMetrics` (UID `victoriametrics`), pointing to
`http://victoria-metrics.monitoring.svc.cluster.local:8428`. The existing
VictoriaLogs datasource and log dashboard are preserved.

Check http://localhost:8428/targets for `support-desk`, `postgres`, and
`slo-prober`. The SLO prober is discovered by Pod label and declared metrics
port, using the existing namespace-scoped discovery RBAC.

## Queries, checks and packaging

`scripts/render-slo-dashboard.py` regenerates the checked-in JSON dashboard,
query catalog and dashboard ConfigMap. Expressions use pure PromQL subqueries;
no vmalert deployment or recording rules are needed. Time-based compliance uses
a fixed 15-second grid, includes missing/stale observations as bad, and requires
all three business ratios, p95 and black-box health to pass. Empty request
windows are shown as zero/insufficient evidence, never as 100% success.

```sh
python3 scripts/render-slo-dashboard.py
python3 -m unittest discover -s scripts -p 'test_slo*.py'
(cd backend && go test -race ./... && go vet ./...)
```

For integration tests, point `TEST_DATABASE_URL` at a **disposable** database.
The existing test harness truncates its tables. Never point it at the lab DB.
