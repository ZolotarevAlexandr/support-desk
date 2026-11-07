#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
./k8s/deploy.sh
./k8s/monitoring/deploy.sh
./k8s/logging/deploy.sh
