# Infrastructure

## Local stack

- [`local/docker-compose.yml`](local/docker-compose.yml): Postgres, Valkey, and the server (optional frontend sidecar on the same container)
- [`docker/Dockerfile`](docker/Dockerfile): runtime image with binary, migrations, and bundled frontend assets

```bash
make up-local
make ps-local
make logs-local
make down-local
make reset-local
```

`make up-local` passes `--env-file` from `.env` or `.env.example`.

URLs:

- Backend: `http://127.0.0.1:8080`
- Frontend sidecar: when `AELORA_FRONTEND_SIDECAR_ENABLED=true`, publish `8081:8081` on the server service in compose if you need browser access from the host

Migrations run at server startup in compose; `make run-migrate-local` is available for manual runs.

## Host mode

Run dependencies via compose, then on the host:

```bash
make run-migrate
make run-server
```

Override Valkey/Postgres hosts in `.env` for localhost when not using in-container hostnames.

## Kubernetes

Base manifests under [`k8s/base`](k8s/base) (deployment, service, config, secret example, migration job). They expose the backend only; the frontend sidecar is for local use.

```bash
make k8s-render
make k8s-apply
make k8s-delete
```

## Operations

- Metrics: `GET /metrics`
- Health: `/livez`, `/readyz` (Valkey + Postgres), `/healthz`
- Migration entrypoint: `aelora-server migrate`
