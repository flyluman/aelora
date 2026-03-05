# Design

## Overview

Aelora is a realtime chat backend:

- One server process (`cmd/server`) handling HTTP + WebSocket.
- Optional local frontend helper listener on a second port of that same server process for same-origin browser use.
- Postgres as source of truth for rooms, members, messages, dedup, outbox.
- Valkey for realtime streams, presence, ACK cursors, membership cache.
- Optional push gateway endpoints for offline delivery.

API style:

- Resource-oriented URLs with path parameters for parent resource scope.
- Uniform JSON response envelope for HTTP APIs: `{success,data,error}`.
- No legacy fallback routes.
- Room IDs use `<8 base62 chars>`.
- Room IDs use a Valkey-backed global sequence and stay unique across restarts and scaled instances.
- Sequence values are obfuscated with keyed Feistel rounds + XOR mixing before base62 encoding.
- Room ID generation is owned by the create-room application use-case, not by transport adapters.

## Architecture

```text
Optional Browser Console
  -> Frontend helper listener (`AELORA_FRONTEND_SIDECAR_ENABLED=true`)
  -> REST + WS
Other Clients (Web/Mobile/Services)
  -> REST + WS
Server (Auth, API, WS Hub, Metrics, Health, Rate Limit)
  -> Postgres (truth: rooms/messages/dedup/outbox)
  -> Valkey (stream replay + event fanout + cache/state)
  -> Push gateway (optional)
```

Core components:

- `frontend`: static control console for local testing
- `internal/adapters/inbound/http/frontend_sidecar.go`: local helper handler used by the optional second listener
- `internal/application`: use-case orchestration
- `internal/adapters/inbound/http`: REST + WS protocol handling
- `internal/adapters/outbound/postgres`: durable persistence
- `internal/adapters/outbound/valkey`: streams/cache/presence/ack/tokens
- `internal/workers`: outbox relay retry loop

REST surface:

- `POST /v1/rooms`
- `PUT /v1/rooms/{room_id}/members/{user_id}`
- `POST /v1/rooms/{room_id}/messages`
- `GET /v1/rooms/{room_id}/messages`
- `PUT /v1/users/me/devices/{device_id}/push-token`
- `DELETE /v1/users/me/devices/{device_id}/push-token`

## Data Model

### Postgres

- `rooms`
- `room_members` (`owner` / `admin` / `member`)
- `messages`
- `message_dedup`
- `message_outbox`

### Valkey

- `stream:room:{room_id}`: per-room replay stream
- `stream:events:message_created`: event fanout bus
- `presence:{user_id}:{device_id}` + `presence_devices:{user_id}`
- `last_ack:{user_id}:{device_id}:{room_id}`
- `room:{room_id}:members`
- `push_tokens:{user_id}`

## Message Lifecycle

1. Validate identity/membership/dedup/rate limits.
2. Persist message in Postgres (durable).
3. Append to Valkey room stream.
4. Publish `message_created` event.
5. On failure paths, enqueue outbox for relay retry (including stream-backfill recovery when needed).
6. Hub broadcasts to connected clients; offline members can get push.
7. Sender does not receive self-echo `message_created` fanout frame.

## Reliability Model

- Outbox relay claims rows with lease and retries.
- Presence refreshed while WS connected.
- Heartbeat ping/pong closes stale sockets.
- ACK cursor is per user+device+room.
- SYNC replays from stream and hydrates reply parents from Postgres.

## Consistency

- Postgres is authoritative.
- Valkey is transport/cache for speed and realtime.
- Membership cache warms from Postgres fallback.
- Room ordering guaranteed only within the same room stream.

## Observability

- `GET /metrics` Prometheus format.
- Request counters + latency timing headers.
- Reliability counters for stream append / publish / outbox fallback events.
- Trace propagation (`X-Trace-ID`, `traceparent`).
- Structured JSON logs use `timestamp` and caller metadata.
- Request/audit logs omit raw query-string values, auth headers, and push-token payloads.
- The frontend helper is optional and local-facing; the base Kubernetes manifests remain backend-only.
- Health:
  - `/livez` liveness
  - `/readyz` dependency readiness
  - `/healthz` readiness alias
