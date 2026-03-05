# Message Journey (Functions + Branches)

This document explains the end-to-end message lifecycle in Aelora, including all major branches in the current implementation.

## 1) High-Level Project Flow

```text
Client (HTTP/WS)
  -> Inbound Adapters (handler/websocket)
    -> Application Use Cases
      -> Postgres (messages, dedup, outbox)
      -> Valkey (room stream, event stream, ack, presence)
      -> Outbox table (fallback path)

Valkey event stream
  -> WebSocket Hub
    -> Connected clients
    -> Push Notifier (offline members)

Outbox Relay Worker
  <- Outbox table
  -> Valkey room stream (if missing)
  -> Valkey event stream publish
```

## 2) Function Map

Main message path functions:

- `internal/adapters/inbound/http/handler.go`
  - `(*Handler).Routes`
- `internal/adapters/inbound/http/crud.go`
  - `(*Handler).sendMessage`
- `internal/adapters/inbound/http/observability.go`
  - `(*Handler).withHTTPLogging`
  - `(*Handler).withRecovery`
- `internal/adapters/inbound/http/frontend_sidecar.go`
  - `NewFrontendSidecarHandler` (optional second listener for local helper traffic)
- `internal/adapters/inbound/http/websocket.go`
  - `(*WebSocketHandler).handleSendMessage`
  - `(*WebSocketHandler).handleSync`
  - `(*WebSocketHandler).handleAck`
- `internal/application/send_message.go`
  - `(*SendMessageUseCase).Execute`
- `internal/adapters/outbound/postgres/message_repo.go`
  - `(*RoomRepository).Save`
  - `(*RoomRepository).ExistsByClientMsgID`
  - `(*RoomRepository).GetByIDs`
- `internal/adapters/outbound/valkey/stream_repo.go`
  - `(*StreamRepository).AppendMessage`
  - `(*StreamRepository).PublishMessageCreated`
  - `(*StreamRepository).ReadFrom`
  - `(*StreamRepository).SubscribeMessageCreated`
- `internal/adapters/inbound/http/hub.go`
  - `(*WebSocketHub).Run`
  - `(*WebSocketHub).broadcastMessageCreated`
- `internal/adapters/outbound/postgres/outbox_repo.go`
  - `(*RoomRepository).Enqueue`
  - `(*RoomRepository).ClaimPending`
  - `(*RoomRepository).MarkStreamAppended`
  - `(*RoomRepository).MarkDispatched`
  - `(*RoomRepository).ReleaseClaim`
- `internal/workers/outbox_relay.go`
  - `(*MessageOutboxRelay).Run`
- `internal/application/sync_messages.go`
  - `(*SyncMessagesUseCase).Execute`
  - `(*SyncMessagesUseCase).recoverAndBackfill`
  - `(*AckMessageUseCase).Execute`

## 3) HTTP Send Journey

Entry: `POST /v1/rooms/{room_id}/messages` -> `(*Handler).sendMessage`

```text
[HTTP sendMessage]
  -> decode JSON
     -> fail: 400 bad request
  -> check room_id in path
     -> missing: 400
  -> resolve user identity
     -> missing: 400
  -> SendMessageUseCase.Execute
     -> error: writeAppError(...) mapped status
     -> success: 201 + message payload
```

## 4) WebSocket Send Journey

Entry: WS message `{"type":"send_message", ...}` -> `(*WebSocketHandler).handleSendMessage`

```text
[WS handleSendMessage]
  -> SendMessageUseCase.Execute
     -> ErrDuplicateMessage: "duplicate client_msg_id"
     -> ErrRateLimited: "rate limit exceeded"
     -> ErrRoomAccessDenied: "room access denied"
     -> ErrReplyTarget: "reply target not found"
     -> domain.ErrInvalidMessage: "invalid message payload"
     -> any other error: "internal error"
     -> success: "message_accepted"
```

## 5) SendMessageUseCase Branches (Core Business Logic)

Function: `(*SendMessageUseCase).Execute`

Ordered behavior and branches:

1. Rate limit check (if limiter exists)
   - pass -> continue
   - fail -> `ErrRateLimited`

2. Dedup pre-check `ExistsByClientMsgID`
   - repo error -> return error
   - already exists -> `ErrDuplicateMessage`
   - not exists -> continue

3. Room + membership check
   - `rooms.Get` error -> return error
   - sender not member -> `ErrRoomAccessDenied`
   - member -> continue

4. Build message object + optional reply check
   - if `ReplyToID != ""`: `messages.GetByIDs`
   - reply lookup error -> return error
   - reply id not found in room -> `ErrReplyTarget`

5. Domain validation `msg.Validate()`
   - invalid -> domain error (`ErrInvalidMessage`)

6. Persist `messages.Save`
   - duplicate at storage layer -> maps to `ErrDuplicateMessage`
   - other error -> return error

7. Append to room stream `stream.AppendMessage`
   - success -> step 8
   - fail:
     - if no outbox configured -> return error
     - enqueue outbox with `streamAppended=false`
       - enqueue fail -> return enqueue error
       - enqueue success -> return `msg, nil` (durable + deferred realtime)

8. Publish event `bus.PublishMessageCreated`
   - success -> return `msg, nil`
   - fail:
     - if no outbox configured -> return error
     - enqueue outbox with `streamAppended=true`
       - enqueue fail -> return enqueue error
       - enqueue success -> return `msg, nil`

## 6) Realtime Fanout Branches

Path: Valkey event stream -> `WebSocketHub.Run` -> `broadcastMessageCreated`

Branches in `broadcastMessageCreated`:

- `rooms.ListMembers` fails -> log + stop fanout for that message.
- For each member:
  - enqueue to each connected socket.
  - if socket queue full -> log warning + unregister client.
  - push notify branch:
    - skip if member has active sockets
    - skip if member is sender
    - skip if push use-case is nil
    - else execute push; if push fails log warning and continue.

## 7) Outbox Relay Branches (Deferred Recovery)

Path: `MessageOutboxRelay.Run` tick loop

```text
[Ticker]
  -> ClaimPending(batch)
     -> claim error: log, next tick
     -> items:
        -> if StreamAppended == false:
           -> AppendMessage
              -> fail: ReleaseClaim, continue
           -> MarkStreamAppended
              -> fail: ReleaseClaim, continue
        -> PublishMessageCreated
           -> fail: ReleaseClaim, continue
        -> MarkDispatched
           -> fail: warn only
        -> next item
```

Important recovery semantics:

- `streamAppended=false` entries attempt stream append first, then event publish.
- `streamAppended=true` entries skip append and only re-publish event.
- Claim lease prevents duplicate active processing across workers.

## 8) SYNC and ACK Branches

### 8.1 SYNC (`(*SyncMessagesUseCase).Execute`)

Branches:

1. room lookup / membership failure -> error.
2. `LastAck` handling:
   - request value present -> use it
   - empty -> load from ack store
   - still empty -> `"0"`
3. limit handling:
   - `<=0` -> default `100`
4. stream read:
   - read error -> error
   - empty result and `msgs != nil` -> `recoverAndBackfill`
5. recovery/backfill:
   - convert streamID to `since` when possible
   - load from Postgres
   - append each recovered message back into room stream
   - append failure at any item -> error
6. reply hydration:
   - no replies needed or no repo -> return empty replies map
   - fetch replies via `GetByIDs`
   - fetch error -> error

### 8.2 ACK (`(*AckMessageUseCase).Execute`)

Branches:

- any empty/blank field (`room_id/user_id/device_id/stream_id`) -> `ErrInvalidAck`
- room get error -> error
- user not room member -> `ErrRoomAccessDenied`
- ack store write error -> error
- success -> ack saved

## 9) Error Mapping Surface

HTTP mapping: `(*Handler).writeAppError`

- duplicate -> `409`
- rate limit -> `429`
- access denied -> `403`
- room not found -> `404`
- room exists -> `409`
- invalid message/room/reply/push payload -> `400`
- default -> `500`

WS mapping: `(*WebSocketHandler).handleSendMessage`, `handleAck`, `handleSync`

- send_message has specific known error strings + generic internal error
- ACK returns `invalid ack payload` or `ack failed`
- SYNC currently returns `sync failed` on any use-case error

## 10) End-to-End Sequence (Happy Path + Fallback Points)

```text
Client -> Inbound Adapter -> SendMessageUseCase
  -> Postgres: dedup + save
  -> Valkey room stream: append
     -> append fails:
        -> Outbox.Enqueue(streamAppended=false)
        -> return success (deferred realtime)
        -> Relay: claim -> append stream -> publish event -> mark dispatched
     -> append succeeds:
        -> Valkey event stream: publish
           -> publish fails:
              -> Outbox.Enqueue(streamAppended=true)
              -> return success (deferred fanout)
              -> Relay: claim -> publish event -> mark dispatched
           -> publish succeeds:
              -> Hub receives event
              -> Hub fanout to WS clients
```
