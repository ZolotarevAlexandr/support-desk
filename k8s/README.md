# Support Desk in Minikube

For the logging assignment, continue with [logging/README.md](logging/README.md).
For the metrics assignment, continue with [monitoring/README.md](monitoring/README.md).
The earlier Kubernetes report is preserved at
[docs/kubernetes/Отчёт.md](../docs/reports/report_week_2_k8s_deployment.md).

Run these commands from the repository root. All manifests explicitly use the
`support-desk` namespace, so the namespace selected during practice does not matter.

## Resources

| File | Resources |
|---|---|
| `namespace.yaml` | Application namespace |
| `configmap.yaml` | Non-secret application settings and database/user names |
| `secret.yaml` | Demo database password, database URL, operator invitation code |
| `postgres.yaml` | 1 GiB PVC, internal Service, one-replica database Deployment |
| `migrate.yaml` | One-time schema migration Job |
| `seed.yaml` | One-time initial category Job |
| `app.yaml` | Application Service and scalable Deployment |

The existing Dockerfile embeds the React UI into the Go server. The application,
migration Job, and seed Job all use `support-desk:metrics-v1`. PostgreSQL uses the same
pinned image as Docker Compose. PostgreSQL uses `Recreate` to avoid overlapping
database Pods sharing the data directory. Scale only `support-app`.

The PVC uses the cluster's default StorageClass. Standard Minikube provides a
default class; verify it with `kubectl --context=minikube get storageclass`.
PostgreSQL 18 mounts its data volume at `/var/lib/postgresql`.

`secret.yaml` deliberately contains public demo values for the local exercise:
database password `support-minikube-demo-password` and operator invitation code
`support-minikube-operator`. It does not use your Compose `.env`.
If you edit the password, update the password in `DATABASE_URL` too. Changing
`POSTGRES_PASSWORD` after database initialization does not change the stored
database password; use SQL to change it, then update the Secret. If you change
the user/database names in the ConfigMap, update the URL accordingly.

## Build the image

For the assignment's `docker-env` workflow, use the Docker container runtime:

```sh
minikube start --driver=docker --container-runtime=docker --cpus=2 --memory=4096
kubectl config use-context minikube
eval "$(minikube docker-env)"
docker build --build-arg VERSION=metrics-v1 -t support-desk:metrics-v1 .
docker pull quay.io/prometheuscommunity/postgres-exporter:v0.20.1
minikube image load support-desk:metrics-v1 quay.io/prometheuscommunity/postgres-exporter:v0.20.1
minikube image ls
```

Building with `docker-env` already makes the image available inside this Docker
runtime; the load command also follows the assignment's explicit load step.
Restore the host Docker environment afterward with
`eval "$(minikube docker-env -u)"`.

If your existing Minikube uses containerd, build with the host Docker daemon
instead and load the image with `minikube image load support-desk:metrics-v1`.

The PostgreSQL Pod now also runs the pinned PostgreSQL exporter on port 9187;
its image must be available to Minikube. Application Pods expose metrics on
9090. These ports are not included in their Services. The monitoring guide
includes the complete image-pull and load commands.

## Deploy in order

After building/loading the image and starting Minikube, run:

```sh
./k8s/deploy.sh
```

The script waits for PostgreSQL, then runs migrations and seeding sequentially,
and finally starts the app. It stops on any failed command. Each invocation
recreates the migration and seed Jobs so previously completed Jobs are not reused.
It can be called from any working directory using its absolute path.

The equivalent manual steps are below. The script includes Job deletion before
these steps to make repeated invocations run them again.

Run one stage at a time. Proceed only after the previous command succeeds.
Do not apply the whole directory at once: Kubernetes does not infer the Compose
`depends_on` sequence from these files.

```sh
kubectl --context=minikube apply -f k8s/namespace.yaml
kubectl --context=minikube apply -f k8s/configmap.yaml -f k8s/secret.yaml
kubectl --context=minikube apply -f k8s/postgres.yaml
kubectl --context=minikube -n support-desk rollout status deployment/postgres --timeout=180s

kubectl --context=minikube apply -f k8s/migrate.yaml
kubectl --context=minikube -n support-desk wait --for=condition=complete job/support-migrate --timeout=150s

kubectl --context=minikube apply -f k8s/seed.yaml
kubectl --context=minikube -n support-desk wait --for=condition=complete job/support-seed --timeout=150s

kubectl --context=minikube apply -f k8s/app.yaml
kubectl --context=minikube -n support-desk rollout status deployment/support-app --timeout=180s
kubectl --context=minikube -n support-desk get pods,deployments,services,jobs,pvc
```

The application does not run migrations or seed data on server startup. Jobs run
`/support migrate` and `/support seed` using the image's entrypoint. Completed
Job Pods remain available for inspecting logs and demonstrating the task.
To repeat a Job, delete it and apply its manifest again:

```sh
kubectl --context=minikube -n support-desk delete job support-migrate
kubectl --context=minikube apply -f k8s/migrate.yaml
kubectl --context=minikube -n support-desk wait --for=condition=complete job/support-migrate --timeout=150s
```

Use the same procedure with `support-seed` and `k8s/seed.yaml` for seeding. When
deploying a new build, update the image tag in all three application manifests.

## Open the UI and scale

Keep this running in a separate terminal:

```sh
kubectl --context=minikube -n support-desk port-forward service/support-app 8080:8080
```

Open http://localhost:8080. If port 8080 is occupied, use `8090:8080` and open
http://localhost:8090. Register a User, create a ticket, and add a comment.
Registering an Operator requires the invitation code from `secret.yaml`.
There are no built-in user accounts.

```sh
kubectl --context=minikube -n support-desk scale deployment/support-app --replicas=3
kubectl --context=minikube -n support-desk rollout status deployment/support-app --timeout=180s
kubectl --context=minikube -n support-desk get pods -o wide
```

All application replicas use the same database and persisted sessions. The
internal `postgres` Service resolves to the database Pod on port 5432. The
`support-app` Service selects ready application Pods on port 8080. Port forwarding
uses one selected application Pod and is sufficient for the UI demonstration.
The app's `/healthz` checks liveness; `/readyz` checks database connectivity and
shutdown state. Reapplying `app.yaml` restores its declared one-replica count.

## Diagnose problems

```sh
kubectl --context=minikube -n support-desk get events --sort-by=.metadata.creationTimestamp
kubectl --context=minikube -n support-desk logs deployment/postgres
kubectl --context=minikube -n support-desk logs job/support-migrate
kubectl --context=minikube -n support-desk logs job/support-seed
kubectl --context=minikube -n support-desk logs deployment/support-app
```

Describe a failing Pod to inspect its events. An unbound PVC requires checking
the default StorageClass. `ImagePullBackOff` for the app requires checking that
the matching image tag was built and loaded into the selected Minikube profile.
Configuration changes require restarting the application to refresh its
environment: `kubectl --context=minikube -n support-desk rollout restart deployment/support-app`.

## Stop workloads

```sh
kubectl --context=minikube delete -f k8s/app.yaml
kubectl --context=minikube delete -f k8s/migrate.yaml -f k8s/seed.yaml --ignore-not-found
kubectl --context=minikube -n support-desk scale deployment/postgres --replicas=0
```

This preserves the PVC for later deployment. Deleting the namespace or PVC can
delete its stored database data. The earlier Kubernetes assignment required a screen recording. The current
logging archive uses the root report and screenshots; see `logging/README.md`.
