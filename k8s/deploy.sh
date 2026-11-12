#!/bin/sh
set -eu

# Resolve manifest paths relative to this script, regardless of the caller's cwd.
cd "$(dirname "$0")/.."

# Namespace and configuration.
kubectl --context=minikube apply -f k8s/namespace.yaml
kubectl --context=minikube apply -f k8s/configmap.yaml -f k8s/secret.yaml

# Create the dedicated synthetic-prober credential once; do not print it.
if ! kubectl --context=minikube -n support-desk get secret support-prober >/dev/null 2>&1; then
  password_file=$(mktemp)
  trap 'rm -f "$password_file"' 0
  chmod 600 "$password_file"
  od -An -N18 -tx1 /dev/urandom | tr -d ' \n' > "$password_file"
  kubectl --context=minikube -n support-desk create secret generic support-prober --from-file=password="$password_file"
  rm -f "$password_file"
  trap - 0
fi

# Start PostgreSQL and wait until it is ready.
kubectl --context=minikube apply -f k8s/postgres.yaml
kubectl --context=minikube -n support-desk rollout status deployment/postgres --timeout=180s

# Recreate Jobs so migrations and seeding run on every invocation.
kubectl --context=minikube -n support-desk delete job support-migrate support-seed --ignore-not-found
kubectl --context=minikube apply -f k8s/migrate.yaml
kubectl --context=minikube -n support-desk wait --for=condition=complete job/support-migrate --timeout=150s
kubectl --context=minikube apply -f k8s/seed.yaml
kubectl --context=minikube -n support-desk wait --for=condition=complete job/support-seed --timeout=150s

# Start the application only after both Jobs succeed.
kubectl --context=minikube apply -f k8s/app.yaml
# The local dev tag is reused; recreate Pods to run the image just loaded.
kubectl --context=minikube -n support-desk rollout restart deployment/support-app
kubectl --context=minikube -n support-desk rollout status deployment/support-app --timeout=180s
kubectl --context=minikube apply -f k8s/slo/prober.yaml
kubectl --context=minikube -n support-desk rollout status deployment/support-prober --timeout=180s
kubectl --context=minikube -n support-desk get pods,deployments,services,jobs,pvc
