#!/bin/sh
# Dashboard-only update; requires the existing SLO/logging stack and admin Secret.
set -eu
cd "$(dirname "$0")/../.."
KUBE_CONTEXT=${KUBE_CONTEXT:-minikube}

# Keep JSON as the source of truth instead of maintaining embedded YAML copies.
dashboard_manifest=$(mktemp)
trap 'rm -f "$dashboard_manifest"' 0
kubectl --context="$KUBE_CONTEXT" -n logging create configmap grafana-app-dashboards \
  --from-file=support-overview.json=docs/grafana/dashboard.json \
  --from-file=support-postgres.json=docs/grafana/postgres-dashboard.json \
  --dry-run=client -o yaml > "$dashboard_manifest"
kubectl --context="$KUBE_CONTEXT" apply -f "$dashboard_manifest"
kubectl --context="$KUBE_CONTEXT" apply -f k8s/slo/grafana-dashboard.yaml \
  -f k8s/logging/grafana-provisioning.yaml -f k8s/logging/grafana.yaml
# Existing provisioning uses subPath mounts; restart to reload those files too.
kubectl --context="$KUBE_CONTEXT" -n logging rollout restart deployment/grafana
kubectl --context="$KUBE_CONTEXT" -n logging rollout status deployment/grafana --timeout=360s
