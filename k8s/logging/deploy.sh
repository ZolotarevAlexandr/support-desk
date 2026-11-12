#!/bin/sh
set -eu
cd "$(dirname "$0")"

kubectl --context=minikube apply -f namespace.yaml

# Keep the password on repeated deployments. Never print or commit it.
if ! kubectl --context=minikube -n logging get secret grafana-admin >/dev/null 2>&1; then
  password_file=$(mktemp)
  trap 'rm -f "$password_file"' 0
  chmod 600 "$password_file"
  od -An -N24 -tx1 /dev/urandom | tr -d ' \n' > "$password_file"
  kubectl --context=minikube -n logging create secret generic grafana-admin \
    --from-file=password="$password_file"
  rm -f "$password_file"
  trap - 0
fi

kubectl --context=minikube apply -f victoria-logs.yaml
kubectl --context=minikube -n logging rollout status deployment/victoria-logs --timeout=180s

kubectl --context=minikube apply -f vector-rbac.yaml -f vector-config.yaml -f vector-daemonset.yaml
kubectl --context=minikube -n logging rollout restart daemonset/vector
kubectl --context=minikube -n logging rollout status daemonset/vector --timeout=180s

../grafana/deploy.sh
kubectl --context=minikube -n logging get deployments,daemonsets,pods,services,pvc
