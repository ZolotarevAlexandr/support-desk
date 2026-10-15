#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p artifacts
compose_file=compose.yaml
if [ -f docker-compose.yaml ]; then compose_file=docker-compose.yaml; fi
# Explicit paths include the source and Kubernetes submission without local environments.
set -- README.md Отчёт.md screencast.mp4 Makefile Dockerfile "$compose_file" \
 .env.example .gitignore .dockerignore backend frontend docs k8s

for submission_item do
 if [ ! -e "$submission_item" ]; then
  printf 'Missing submission file or directory: %s\n' "$submission_item" >&2
  exit 1
 fi
done

# Keep the previous archive intact if packaging fails.
submission_archive_tmp=$(mktemp artifacts/.support-desk-source.XXXXXX.tar.gz)
trap 'rm -f "$submission_archive_tmp"' 0
tar --exclude='node_modules' --exclude='dist' --exclude='*.tsbuildinfo' \
 --exclude='test-results' --exclude='playwright-report' \
 --exclude='.env' --exclude='.env.local' --exclude='.env.production' \
 --exclude='.env.development' \
 -czf "$submission_archive_tmp" "$@"
mv "$submission_archive_tmp" artifacts/support-desk-source.tar.gz
printf '%s\n' 'Created artifacts/support-desk-source.tar.gz'
