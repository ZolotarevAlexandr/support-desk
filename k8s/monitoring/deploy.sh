#!/bin/sh
set -eu
cd "$(dirname "$0")"

HELM=${HELM:-helm}
KUBE_CONTEXT=${KUBE_CONTEXT:-minikube}

kubectl --context="$KUBE_CONTEXT" apply -f ../namespace.yaml -f namespace.yaml -f discovery-rbac.yaml
"$HELM" upgrade --install vm-single victoria-metrics-single \
  --repo https://victoriametrics.github.io/helm-charts/ \
  --version 0.49.0 --kube-context "$KUBE_CONTEXT" --namespace monitoring \
  --values values.yaml --wait --timeout 180s
# The chart does not add a ConfigMap checksum to its Pod template. Restart to
# load changed scrape configuration on repeated deployments as well.
kubectl --context="$KUBE_CONTEXT" -n monitoring rollout restart statefulset/victoria-metrics
kubectl --context="$KUBE_CONTEXT" -n monitoring rollout status statefulset/victoria-metrics --timeout=180s
kubectl --context="$KUBE_CONTEXT" -n monitoring get pods,services,pvc
