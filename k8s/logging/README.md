Run from the repository root. The project uses plain Kubernetes manifests: one
VictoriaLogs instance with a PVC, a Vector DaemonSet, and Grafana with the official
VictoriaLogs plugin. The application and PostgreSQL remain in `support-desk`;
observability resources run in `logging` in the same cluster.

## Build and load images

Requires Docker, Minikube, and kubectl. Reuse the existing profile and its data:

```sh
minikube start --profile=minikube
kubectl --context=minikube get nodes,storageclass
docker build --build-arg VERSION=dev -t support-desk:dev .
docker pull victoriametrics/victoria-logs:v1.52.0
docker pull timberio/vector:0.51.1-debian
docker pull grafana/grafana:12.2.0
docker pull postgres:18.0-alpine3.22@sha256:48c8ad3a7284b82be4482a52076d47d879fd6fb084a1cbfccbd551f9331b0e40
minikube image load support-desk:dev victoriametrics/victoria-logs:v1.52.0 timberio/vector:0.51.1-debian grafana/grafana:12.2.0 postgres:18.0-alpine3.22@sha256:48c8ad3a7284b82be4482a52076d47d879fd6fb084a1cbfccbd551f9331b0e40
```

Use the host Docker daemon for these commands. If your shell previously used
`minikube docker-env`, restore it with `eval "$(minikube docker-env -u)"` first.
Loading the images through the host also works when the node cannot reach an
image registry directly. Grafana downloads the pinned plugin `0.32.0` when it
starts, so the pod needs access to Grafana's plugin download service.

## Deploy

```sh
./k8s/logging/deploy.sh
./k8s/deploy.sh
kubectl --context=minikube -n support-desk scale deployment/support-app --replicas=3
kubectl --context=minikube -n support-desk rollout status deployment/support-app --timeout=180s
```

The logging script creates a random Grafana admin password in the `grafana-admin`
Secret only if it does not already exist. The application script starts
PostgreSQL, runs migrations and idempotent seeding, then starts the server.
It uses the repository's public local-demo application Secret. Existing
PostgreSQL data and the VictoriaLogs PVC are preserved. `app.yaml` declares one
replica; the scale command reproduces the three replicas used for verification.

The logging script restarts Vector and Grafana to load ConfigMap changes. On
repeated deployment, VictoriaLogs configuration changes that do not modify the
pod template require their own rollout; the provided storage settings are flags
in the pod template. Do not apply the whole `k8s/` directory recursively: the
application Jobs have a required execution order.

## Open the application and logging tools

Keep each port-forward in its own terminal:

```sh
kubectl --context=minikube -n support-desk port-forward service/support-app 18080:8080
kubectl --context=minikube -n logging port-forward service/victoria-logs 9428:9428
kubectl --context=minikube -n logging port-forward service/grafana 3000:3000
```

- Application: http://localhost:18080
- vmui: http://localhost:9428/select/vmui/
- Grafana: http://localhost:3000/d/support-logs/support-desk-application-logs

VictoriaLogs v1.52.0 exposes `/select/vmui/`; the `/vmui/` shortcut in the assignment
does not exist in this version. Its root page links to the actual UI.
Reconnect a port-forward if its selected pod is replaced.

Grafana username is `admin`. Its password is stored in the Kubernetes Secret
`grafana-admin`; retrieve it with:

```sh
kubectl --context=minikube -n logging get secret grafana-admin -o jsonpath='{.data.password}' | base64 --decode
```

The datasource and dashboard are provisioned by
[grafana-provisioning.yaml](grafana-provisioning.yaml). Grafana uses `emptyDir`;
its admin password, datasource, and dashboard are restored from the Secret and
ConfigMap after a restart. Unprovisioned UI edits are not persisted.

## Generate traffic and run queries

```sh
python3 scripts/logging-traffic.py
```

This registers or reuses the default demo account
`logging-lab@example.test` / `support-minikube-demo-password`, creates a ticket and
comment, performs successful reads, and generates 401 responses and intentional
500 responses from `/debug/error` without cookies. Each round produces a WARN
access event for the 401 and an ERROR access event for the 500. Rebuild and deploy
the application after adding the endpoint before running this script.
The script first checks `/debug/error` once before creating demo data, so 30
rounds produce 31 intentional ERROR events. If the endpoint returns the frontend
HTML instead of HTTP 500, the script stops with an image-update instruction.

The application image includes `/debug/error`. When updating an already running
cluster, load the new image and restart the Deployment to refresh the reused dev tag:

```sh
docker build --build-arg VERSION=dev -t support-desk:dev .
minikube image load support-desk:dev
kubectl --context=minikube -n support-desk set image deployment/support-app app=support-desk:dev
kubectl --context=minikube -n support-desk rollout restart deployment/support-app
kubectl --context=minikube -n support-desk rollout status deployment/support-app --timeout=180s
```

Reconnect the application port-forward after its selected pod is replaced.
It prints request IDs and statuses, never session cookies. Each run leaves a
ticket and comment. Python 3 uses only the standard library.

Open vmui, paste the four queries from the
[root report](../../docs/reports/report_week_3_logs.md#четыре-запроса-в-vmui), and execute them. Use the JSON
view to show all returned fields. `timestamp` maps to `_time`, `message` maps to
`_msg`, and workload identity fields form `_stream`. Application 4xx access events
have level `WARN`, 5xx have `ERROR`, and successful requests have `INFO`.

To inspect the intentional errors, run:

```text
_time:15m service.name:="support-desk" level:=ERROR -path:("/healthz" OR "/readyz")
```

All four lab queries and every Grafana panel exclude `/healthz` and `/readyz`.
Kubernetes continues running its probes, and their records remain stored for
diagnostics. They no longer displace the generated traffic in these views.

The `_time:15m` filter controls the query window; vmui shows a notice that it
overrides the manual time picker. Set its separate **Limit** field to **200** for
the raw-record query, which also allows 200 rows. A 30-round script run produces
126 events, so the entire run fits. A default limit of 50 only shows the latest
50 matches; omitted records are still stored. Generate fresh traffic if the
demo has aged out of the selected time window.

Take screenshots manually in the browser after executing each query in vmui,
showing the query, results, and time interval. Also capture the provisioned
Grafana dashboard after signing in with `admin` and the password from the
`grafana-admin` Secret. Save the images under `docs/logging/screenshots/` using
the filenames referenced by the root report. The existing five screenshots are
included as lab evidence.

## Configuration and diagnostics

### Clear stored logs before another run

For this machine's Minikube profile, the VictoriaLogs PVC uses the node directory
`/tmp/hostpath-provisioner/logging/victoria-logs-data`. To permanently clear its
log history, stop VictoriaLogs before clearing that directory, then restart it:

```sh
kubectl --context=minikube -n logging scale deployment/victoria-logs --replicas=0
kubectl --context=minikube -n logging wait --for=delete pod -l app=victoria-logs --timeout=120s
minikube ssh -- 'sudo find /tmp/hostpath-provisioner/logging/victoria-logs-data -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +'
kubectl --context=minikube -n logging scale deployment/victoria-logs --replicas=1
kubectl --context=minikube -n logging rollout status deployment/victoria-logs --timeout=180s
```

This preserves PostgreSQL and Vector checkpoints. Old container files are not
replayed from the beginning; new events continue arriving. Reconnect the
VictoriaLogs port-forward afterward, then run the traffic script again. Deleting
and recreating the PVC alone may reuse the same Minikube host directory and leave
the old records in place.

### Resources and diagnostics

| File | Resources / purpose |
|---|---|
| `namespace.yaml` | Observability namespace |
| `victoria-logs.yaml` | Deployment, Service, 2 GiB PVC, 7-day retention |
| `vector-rbac.yaml` | ServiceAccount and read-only discovery RBAC |
| `vector-config.yaml` | Kubernetes source, JSON/text normalization, ES bulk sink, disk buffer |
| `vector-daemonset.yaml` | One collector per node, read-only node log mounts, persistent checkpoints |
| `grafana.yaml` | Deployment and Service, plugin installation, Secret reference |
| `grafana-provisioning.yaml` | Official datasource and application dashboard |

Vector selects only `support-app`, `postgres`, `support-migrate`, and `support-seed`
in `support-desk`. The source handles Docker/CRI envelopes before the transform
parses application JSON. PostgreSQL text output is wrapped with the runtime time,
service/pod identity, and recognized severity; unknown severities stay `UNKNOWN`.
`source_format` distinguishes native JSON from wrapped text. Unparseable app
records are retained with `parse_error=true`. VictoriaLogs/Grafana/Vector logs are
excluded from collection.

Minikube's Docker runtime uses symlinks from `/var/log/pods` into
`/var/lib/docker/containers`, so both are mounted read-only. Vector runs as root
to read these node files, with capabilities dropped and privilege escalation
disabled. Checkpoints and a bounded disk buffer (~256 MiB) live under node-local
`/var/lib/support-logs-vector`. Full buffers apply backpressure; source files can
still rotate before collection during a long outage. This is not exactly-once
delivery. The PVC and node-local files survive pod replacement, not deletion of
the Minikube node/profile.

```sh
kubectl --context=minikube -n logging get pods,deployments,daemonsets,services,pvc
kubectl --context=minikube -n logging logs daemonset/vector --tail=50
kubectl --context=minikube -n logging logs deployment/victoria-logs --tail=50
kubectl --context=minikube -n logging logs deployment/grafana --tail=50
kubectl --context=minikube -n logging get events --sort-by=.metadata.creationTimestamp
curl -fsS http://localhost:9428/select/logsql/query --data-urlencode 'query=_time:15m | stats by (service.name, k8s.container.name) count() as events'
```

App logs intentionally exclude raw driver error payloads, request bodies, headers,
query strings, passwords, cookies, and invitation codes. Unexpected errors retain
their type and request ID for correlation. PostgreSQL output is a separate source
and may contain database diagnostics; application error redaction does not apply
to database-server logging.

## Package the submission

Archive creation is left to the user. Check that the evidence files are present
without creating an archive:

```sh
./scripts/package-logs.sh --check
```

When ready to create it, run:

```sh
./scripts/package-logs.sh
```

The result is `artifacts/support-desk-logs.tar.gz`, with root `report_week_2_k8s_deployment.md`, all five
screenshots, manifests/configuration, current source, and reproduction scripts.
The archive excludes `.env` files, local credentials, dependencies, compiled
assets, assignment materials, and the previous recording. The earlier report is
preserved at [report_week_2_k8s_deployment.md](../../docs/reports/report_week_2_k8s_deployment.md).
