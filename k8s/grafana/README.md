# Grafana lab

Reuse the existing Grafana 12.2 in `logging`, VictoriaMetrics in `monitoring`,
PostgreSQL Exporter 0.20.1 and the SLO prober. The VictoriaMetrics datasource
uses the built-in Prometheus plugin, UID `victoriametrics`, and URL
`http://victoria-metrics.monitoring.svc.cluster.local:8428`. No extra plugin is needed.

## Deploy

From the repository root, using the host Docker daemon:

```sh
minikube start --profile=minikube
docker build --build-arg VERSION=dev -t support-desk:dev .
minikube image load support-desk:dev
./k8s/deploy.sh
# Existing installation: update only Grafana dashboards/provisioning.
./k8s/grafana/deploy.sh
```

For a fresh installation, build/load `support-prober:slo-v1` and install the
monitoring/logging stacks using the [SLO guide](../slo/README.md). The logging
deployment now also provisions the new dashboards. Existing database PVCs,
Grafana admin Secret and prober credential Secret are reused.

`deploy.sh` creates a ConfigMap directly from the two JSON files in
`docs/grafana`, applies existing provisioning and restarts Grafana. It can be
rerun without restarting the application or database. `KUBE_CONTEXT` defaults
to `minikube`. Grafana stores its working database in `emptyDir`; dashboard JSON
is provisioned again after restart, so edit/export the files instead of relying
on UI-only changes. The earlier logs and SLO dashboards remain provisioned.

In separate terminals:

```sh
kubectl --context=minikube -n logging port-forward service/grafana 13000:3000
kubectl --context=minikube -n monitoring port-forward service/victoria-metrics 8428:8428
kubectl --context=minikube -n support-desk port-forward service/support-app 18080:8080
```

Use the existing Grafana admin login. The SLO guide explains how to copy its
password privately. Open:

- Application overview: http://localhost:13000/d/support-overview
- Imported PostgreSQL dashboard: http://localhost:13000/d/support-postgres
- Detailed SLO dashboard: http://localhost:13000/d/support-slo
- Scrape targets: http://localhost:8428/targets

Reconnect a port-forward when its selected Pod is replaced.

## Metrics and dashboard behavior

The overview contains HTTP RED metrics, scrape/database health, service-wide
prober SLA and runtime utilization/concurrency. The application Pod selector
affects application HTTP/runtime panels, while prober/SLA and database panels
remain service-wide. Refresh is 15s, matching the scrape interval.

The new HTTP families are:

| Metric | Labels | Meaning |
|---|---|---|
| `support_http_requests_total` | `method`, `route`, `status` | Completed API and `/debug/error` responses |
| `support_http_request_duration_seconds` | `method`, `route` | Handler duration in seconds, including failed responses |
| `support_http_requests_in_flight` | none | Concurrent measured requests |

Routes use Gin templates, with `unmatched` for unknown API routes. Nonstandard
HTTP methods map to `OTHER`. Health checks, assets and metrics scraping are
excluded. The existing outer cross-origin protection rejects requests before
Gin; those rejections are outside the HTTP metric scope. Runtime collectors
remain on the internal port 9090, separate from the application Service.

Rates are calculated per counter series before aggregation across replicas;
the histogram is aggregated by `le` before p95 calculation. HTTP 5xx shows zero
when observed traffic has no 5xx, and no traffic/no data when the denominator
is zero. Synthetic p95 includes network effects; HTTP p95 measures the handler.

The SLA expression is copied unchanged from `docs/slo/queries.json`: fixed 15s
intervals, rolling 30m, target 99%. Read its value together with window coverage.
Allow at least 30 minutes of healthy observations before calling it a full
baseline. Recovery of the latest probe, five-minute SLIs and 30-minute SLA occurs
at different times.

The PostgreSQL dashboard comes from the exporter's tagged
[v0.20.1 dashboard](https://github.com/prometheus-community/postgres_exporter/blob/v0.20.1/postgres_mixin/dashboards/postgres-overview.json).
Changes: bind the datasource, set stable UID/default variables, remove the three
legacy backend/checkpoint buffer targets unavailable on PostgreSQL 18, correct
conflict/deadlock legends, and clarify that QPS is transactions/second and
`numbackends` includes idle connections. No custom collectors are required.

## Verify and record

```sh
python3 scripts/metrics-traffic.py --rounds 4
curl -fsSG http://localhost:8428/api/v1/query \
  --data-urlencode 'query=sum by (job) (up)'
curl -fsSG http://localhost:8428/api/v1/query \
  --data-urlencode 'query=pg_up{job="postgres"}'
curl -fsSG http://localhost:8428/api/v1/query \
  --data-urlencode 'query=sum by (status) (rate(support_http_requests_total{job="support-desk"}[5m]))'
```

Check all panels, select one application Pod, return to All, and follow the
PostgreSQL link. Database transaction/row panels should respond to traffic.
Verify persistence by rerunning the dashboard deployment and reopening all four
dashboards. Export the application dashboard using Grafana's Export JSON action;
use the classic dashboard JSON format, not an HTTP API response envelope.

To collect failure/recovery evidence, leave the prober running and use:

```sh
./k8s/slo/faults.sh on
# Observe errors/latency and capture the selected interval.
./k8s/slo/faults.sh off
```

Record a 2–3 minute walkthrough of overview → HTTP/runtime → SLA/prober →
PostgreSQL. Use the recorded historical interval to show degradation/recovery
without waiting through the rolling window in the video. Screenshots and JSON
are in [docs/grafana](../../docs/grafana); see the
[Russian report](../../docs/reports/report_week_7_grafana.md).
Save the submitted recording as `screencast.mp4` in the repository root.

Backend verification: `go test -race ./...` and `go vet ./...` from `backend/`.
Set `TEST_DATABASE_URL` only to a disposable test database; integration tests
truncate its tables. Dashboard provisioning can be validated without changing
the cluster using `kubectl create configmap ... --dry-run=client -o yaml` as in
the deploy script.
