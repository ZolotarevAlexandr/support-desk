#!/bin/sh
set -eu
cd "$(dirname "$0")"
if [ -f ../../.env ]; then . ../../.env; fi
KUBE_CONTEXT=${KUBE_CONTEXT:-minikube}

kubectl --context="$KUBE_CONTEXT" apply -f ../monitoring/namespace.yaml
if [ -n "${TELEGRAM_BOT_TOKEN:-}" ]; then
  printf '%s' "$TELEGRAM_BOT_TOKEN" |
    kubectl --context="$KUBE_CONTEXT" -n monitoring create secret generic alertmanager-telegram \
      --from-file=bot-token=/dev/stdin --dry-run=client -o yaml |
    kubectl --context="$KUBE_CONTEXT" -n monitoring apply -f -
fi
kubectl --context="$KUBE_CONTEXT" -n monitoring get secret alertmanager-telegram >/dev/null

kubectl --context="$KUBE_CONTEXT" -n monitoring create configmap alertmanager-config \
  --from-file=alertmanager.yml --dry-run=client -o yaml |
  kubectl --context="$KUBE_CONTEXT" -n monitoring apply -f -
kubectl --context="$KUBE_CONTEXT" -n monitoring create configmap support-alert-rules \
  --from-file=business.yaml --from-file=runtime.yaml=runtime-normal.yaml \
  --dry-run=client -o yaml |
  kubectl --context="$KUBE_CONTEXT" -n monitoring apply -f -

kubectl --context="$KUBE_CONTEXT" apply -f stack.yaml
# Full deployment: reload rules, public config and token; PVCs are retained.
# For demo recovery, update only rules and restart vmalert (see README).
kubectl --context="$KUBE_CONTEXT" -n monitoring rollout restart deployment/alertmanager deployment/vmalert
kubectl --context="$KUBE_CONTEXT" -n monitoring rollout status deployment/alertmanager --timeout=180s
kubectl --context="$KUBE_CONTEXT" -n monitoring rollout status deployment/vmalert --timeout=180s
