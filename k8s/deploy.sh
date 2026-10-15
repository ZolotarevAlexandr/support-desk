#!/bin/sh
set -eu

# Resolve manifest paths relative to this script, regardless of the caller's cwd.
cd "$(dirname "$0")/.."

# Namespace and configuration.
kubectl --context=minikube apply -f k8s/namespace.yaml
kubectl --context=minikube apply -f k8s/configmap.yaml -f k8s/secret.yaml

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
kubectl --context=minikube -n support-desk rollout status deployment/support-app --timeout=180s
kubectl --context=minikube -n support-desk get pods,deployments,services,jobs,pvc
