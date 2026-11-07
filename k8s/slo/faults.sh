#!/bin/sh
set -eu
case "${1:-}" in
  on) rate=0.20; delay=1.5s ;;
  off) rate=0; delay=0s ;;
  *) echo 'Usage: k8s/slo/faults.sh on|off' >&2; exit 2 ;;
esac
kubectl --context=minikube -n support-desk set env deployment/support-app "LAB_FAULT_RATE=$rate" "LAB_FAULT_DELAY=$delay"
kubectl --context=minikube -n support-desk rollout status deployment/support-app --timeout=180s
