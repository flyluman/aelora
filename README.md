# Aelora

Realtime chat backend in Go: REST and WebSocket APIs, Postgres persistence, Valkey streams and cache.

## Features

- Durable messages with client-side dedup and per-sender rate limits
- WebSocket fanout with SYNC replay and per-device ACK cursors
- Transactional outbox when stream append or event publish fails
- Room membership roles (owner, admin, member) and server-generated room IDs
- Optional HTTP push gateway for offline devices
- Prometheus metrics, structured logs, trace ID propagation, health checks

## Quick start

**Requirements:** Docker, Go 1.26+

```bash
cp .env.example .env
make up-local
make test
```

- API: `http://127.0.0.1:8080`
- Dev console (optional): set `AELORA_FRONTEND_SIDECAR_ENABLED=true`, publish port `8081` on the server service in compose, then open `http://127.0.0.1:8081/` (see [infra/README.md](infra/README.md))

Local auth is off by default (`AELORA_AUTH_ENABLED=false`); use `X-User-ID` for REST and `user_id` / `device_id` query params for WebSocket. See [docs/guide.md](docs/guide.md) for production auth and full API behavior.

## Configuration

Environment variables are documented in [.env.example](.env.example). Compose loads `.env` (or `.env.example` if `.env` is missing) via `make up-local`.

## Documentation

| Doc | Description |
|-----|-------------|
| [docs/design.md](docs/design.md) | Architecture and data model |
| [docs/guide.md](docs/guide.md) | Client integration (HTTP + WebSocket) |
| [docs/message_journey.md](docs/message_journey.md) | Send/sync/outbox code paths |
| [docs/back_of_envelope.md](docs/back_of_envelope.md) | Capacity planning worksheet |
| [docs/runbook_backup_restore.md](docs/runbook_backup_restore.md) | Backup and restore |
| [api/openapi.yaml](api/openapi.yaml) | OpenAPI spec |
| [frontend/README.md](frontend/README.md) | Browser dev console |
| [infra/README.md](infra/README.md) | Docker Compose and Kubernetes |

## Project layout

```
cmd/server/                 Entrypoint (serve, migrate)
internal/domain/            Entities and validation
internal/application/       Use cases
internal/ports/             Interfaces
internal/adapters/inbound/http/   REST, WebSocket, optional frontend sidecar
internal/adapters/outbound/postgres/   Rooms, messages, outbox
internal/adapters/outbound/valkey/     Streams, presence, ACK, cache
internal/adapters/outbound/roomcache/  Postgres + Valkey room cache
internal/platform/          Config, auth, migrations, telemetry, health
internal/workers/           Outbox relay
migrations/postgres/        SQL migrations
frontend/                   Static dev console
infra/                      Dockerfile, compose, k8s base manifests
```

## Development

```bash
make fmt
make lint
make vet
make build
make image-server
```

Optional Postgres integration tests:

```bash
export AELORA_TEST_POSTGRES_DSN='postgres://postgres:postgres@127.0.0.1:5432/aelora?sslmode=disable'
go test ./...
```

Kubernetes helpers: `make k8s-render`, `make k8s-apply`, `make k8s-delete`.

## API overview

| Method | Path |
|--------|------|
| GET | `/livez`, `/readyz`, `/healthz`, `/metrics` |
| POST | `/v1/rooms`, `/v1/rooms/{room_id}/messages` |
| PUT | `/v1/rooms/{room_id}/members/{user_id}` |
| GET | `/v1/rooms/{room_id}/messages` |
| PUT/DELETE | `/v1/users/me/devices/{device_id}/push-token` |
| GET | `/v1/ws` |

JSON responses use `{ "success": true, "data": ... }` or `{ "success": false, "error": { "code", "message" } }` (except `/metrics`).
