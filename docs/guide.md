# Guide

Integration guide for HTTP and WebSocket clients.

## 1. Local Runtime

Start local stack:

```bash
make up-local
```

The Docker-first defaults in `.env.example` now match the local compose stack.

Enable the frontend helper in `.env`:

```bash
AELORA_FRONTEND_SIDECAR_ENABLED=true
```

Then run:

```bash
make up-local
```

Services:

- Server: `127.0.0.1:8080`
- Valkey: `valkey:6379` inside compose; use `127.0.0.1:6379` in `.env` when running the server on the host against a local Valkey
- Postgres: `127.0.0.1:5432`
- Frontend helper listener: `127.0.0.1:8081` when enabled from the same server process

Useful local commands:

```bash
make ps-local
make logs-local
make run-migrate-local
make reset-local
make test
```

Browser console:

- Set `AELORA_FRONTEND_SIDECAR_ENABLED=true` in `.env`
- Open `http://127.0.0.1:8081/`
- The console defaults its Base URL to the current origin, so browser requests stay same-origin through the helper listener.
- The backend no longer serves the frontend at `/frontend/`.

## 2. Identity and Auth

### Production mode (`AELORA_AUTH_ENABLED=true`)

Required for protected routes:

- `Authorization: Bearer <access_token>`
- `X-Device-ID: <stable-device-id>`

Identity source:

- Backend user id is token `sub`.

### Local mode (`AELORA_AUTH_ENABLED=false`)

- REST uses `X-User-ID`.
- WS uses query params: `?user_id=<id>&device_id=<id>`.

## 3. Room IDs and URL Design

- Room IDs are server-generated IDs in the form `<8 base62 chars>`.
- IDs come from a Valkey-backed global sequence (safe across restarts and horizontal scaling).
- Sequence values are obfuscated through keyed Feistel rounds with XOR mixing, so IDs look random.
- ID generation is handled in the create-room application use-case (business layer), not in HTTP handlers.
- Never assume business meaning from room IDs.
- Treat room IDs as sensitive identifiers and store them client-side only as needed.

## 4. Canonical HTTP API

Response envelope (all JSON HTTP APIs except `/metrics`):

```json
{
  "success": true,
  "data": {}
}
```

Error envelope:

```json
{
  "success": false,
  "error": {
    "code": "invalid_request",
    "message": "human readable message"
  }
}
```

Health and metrics:

- `GET /livez`
- `GET /readyz`
- `GET /healthz`
- `GET /metrics`

Room and message resources:

- `POST /v1/rooms`
- `PUT /v1/rooms/{room_id}/members/{user_id}`
- `POST /v1/rooms/{room_id}/messages`
- `GET /v1/rooms/{room_id}/messages?limit=<n>&since=<RFC3339>`

Push token resources:

- `PUT /v1/users/me/devices/{device_id}/push-token`
- `DELETE /v1/users/me/devices/{device_id}/push-token`

Strict routing rules:

- Message create uses path `room_id` only.
- Membership add uses path `user_id` only.
- Push token routes use path `device_id` only.
- Unknown JSON fields are rejected (`400`).

## 5. HTTP Flows (Detailed)

### 5.1 Create room

Request:

```bash
curl -i -X POST http://127.0.0.1:8080/v1/rooms \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: u1' \
  -d '{"member_ids":["u2","u3"]}'
```

Response example:

```json
{
  "success": true,
  "data": {
    "room_id": "0aZ9Kp2Q",
    "members": ["u1", "u2", "u3"],
    "actor": "u1"
  }
}
```

### 5.2 Add room member

Request:

```bash
curl -i -X PUT \
  http://127.0.0.1:8080/v1/rooms/0aZ9Kp2Q/members/u4 \
  -H 'X-User-ID: u1'
```

Authorization behavior:

- Caller adding another user must be `owner` or `admin`.
- Existing member add is idempotent and returns success.

Response example:

```json
{
  "success": true,
  "data": {
    "room_id": "0aZ9Kp2Q",
    "members": ["u1", "u2", "u3", "u4"],
    "actor": "u1",
    "added_user_id": "u4"
  }
}
```

### 5.3 Send room message

Request:

```bash
curl -i -X POST \
  http://127.0.0.1:8080/v1/rooms/0aZ9Kp2Q/messages \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: u1' \
  -d '{"content":"hello","client_msg_id":"c-1"}'
```

Optional reply payload:

```json
{
  "content": "reply message",
  "client_msg_id": "c-2",
  "reply_to_message_id": "01JXYZ..."
}
```

Success response shape:

```json
{
  "success": true,
  "data": {
    "message_id": "06EP4VRMFSNY44A1YD9JY3T0P4",
    "client_msg_id": "c-1",
    "room_id": "0aZ9Kp2Q",
    "sender_id": "u1",
    "content": "hello",
    "reply_to_message_id": "",
    "created_at": "2026-04-06T10:15:00Z"
  }
}
```

### 5.4 List room messages

Request:

```bash
curl -i \
  "http://127.0.0.1:8080/v1/rooms/0aZ9Kp2Q/messages?limit=50" \
  -H 'X-User-ID: u1'
```

Query behavior:

- `limit` default is `50`, capped at `500`.
- `since` must be RFC3339 when present.

Response shape:

```json
{
  "success": true,
  "data": {
    "room_id": "0aZ9Kp2Q",
    "messages": [],
    "count": 0,
    "limit": 50
  }
}
```

### 5.5 Register and delete push token

Register:

```bash
curl -i -X PUT \
  http://127.0.0.1:8080/v1/users/me/devices/phone2/push-token \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: u2' \
  -d '{"platform":"android","token":"android-demo-token"}'
```

Delete:

```bash
curl -i -X DELETE \
  http://127.0.0.1:8080/v1/users/me/devices/phone2/push-token \
  -H 'X-User-ID: u2'
```

Register response example:

```json
{
  "success": true,
  "data": {
    "user_id": "u2",
    "device_id": "phone2",
    "platform": "android",
    "action": "registered"
  }
}
```

Delete response example:

```json
{
  "success": true,
  "data": {
    "user_id": "u2",
    "device_id": "phone2",
    "action": "deleted"
  }
}
```

## 6. WebSocket Contract (Detailed)

Connect (local mode):

```text
ws://127.0.0.1:8080/v1/ws?user_id=u2&device_id=phone2
```

When using the browser console helper listener, the same endpoint is available at:

```text
ws://127.0.0.1:8081/v1/ws?user_id=u2&device_id=phone2
```

Connect (auth enabled):

- URL: `wss://<host>/v1/ws`
- Headers: `Authorization`, `X-Device-ID`

Inbound WS message types:

- `send_message`
- `ACK`
- `SYNC`

Outbound WS message types:

- `message_accepted`
- `message_created`
- `ack_saved`
- `sync_result`
- `error`

### 6.1 `send_message`

Request:

```json
{
  "type": "send_message",
  "room_id": "0aZ9Kp2Q",
  "content": "hello from websocket",
  "client_msg_id": "c-ws-1",
  "reply_to_message_id": ""
}
```

Expected sequence:

1. `message_accepted`
2. `message_created` (fanout event)

`message_accepted` example:

```json
{
  "type": "message_accepted",
  "message": {
    "message_id": "06EP4VRMFSNY44A1YD9JY3T0P4",
    "room_id": "0aZ9Kp2Q",
    "sender_id": "u1",
    "client_msg_id": "c-ws-1",
    "reply_to_message_id": "",
    "created_at": "2026-04-06T10:15:00Z"
  }
}
```

### 6.2 `SYNC`

Request:

```json
{
  "type": "SYNC",
  "room_id": "0aZ9Kp2Q",
  "last_ack": "0",
  "limit": 100
}
```

Response:

```json
{
  "type": "sync_result",
  "message": {
    "room_id": "0aZ9Kp2Q",
    "messages": [],
    "replies": {},
    "count": 0
  }
}
```

Notes:

- `replies` contains hydrated parent messages for reply rendering.
- If stream replay is empty, server may recover from durable history path.

### 6.3 `ACK`

Request:

```json
{
  "type": "ACK",
  "room_id": "0aZ9Kp2Q",
  "stream_id": "1700000000200-0"
}
```

Response:

```json
{
  "type": "ack_saved",
  "message": {
    "stream_id": "1700000000200-0"
  }
}
```

## 7. Error Model

HTTP:

- `400` invalid payload / validation errors
- `401` unauthorized
- `403` room access denied
- `404` room not found
- `409` duplicate conflicts
- `429` rate limit
- `500` internal error

WebSocket:

- `{"type":"error","error":"..."}` for invalid requests/access/rate limits/internal failures.

## 8. Client Best Practices

- Generate unique `client_msg_id` for every outbound message.
- Keep per-device ACK cursor per room.
- Run `SYNC` on reconnect before considering timeline complete.
- Deduplicate rendered messages by message id.
- Handle `message_created` and `sync_result` through the same merge path.

## 9. Related Docs

- Architecture/design: `docs/design.md`
- Ops backup/restore: `docs/runbook_backup_restore.md`
