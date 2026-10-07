#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p artifacts
compose_file=compose.yaml
if [ -f docker-compose.yaml ]; then compose_file=docker-compose.yaml; fi
# Explicit source paths prevent credentials, database files, or tool caches entering the submission.
tar --exclude='node_modules' --exclude='dist' --exclude='*.tsbuildinfo' \
 --exclude='test-results' --exclude='playwright-report' \
 -czf artifacts/support-desk-source.tar.gz \
 README.md Отчёт.md plan.md task.md Makefile Dockerfile "$compose_file" \
 .env.example .gitignore .dockerignore backend frontend scripts docs
printf '%s\n' 'Created artifacts/support-desk-source.tar.gz'
