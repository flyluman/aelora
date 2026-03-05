# Back-of-Envelope: Full Service Capacity

Purpose: estimate end-to-end capacity of Aelora (HTTP + WS + Valkey + Postgres + outbox + push), not only stream retention.

This is a planning worksheet, not a benchmark result.

## 1) Current Runtime Defaults

- `AELORA_SEND_RATE_LIMIT_PER_SECOND=30` (per sender key, in-memory limiter)
- `AELORA_VALKEY_ROOM_STREAM_MAXLEN=20000`
- `AELORA_VALKEY_EVENT_STREAM_MAXLEN=50000`
- `AELORA_PUSH_HTTP_TIMEOUT_MS=2000`
- Outbox relay tick: every 2s, batch 200 (`cmd/server/main.go`)
- Stream trim mode: `XADD MAXLEN ~ N` (approximate)
- Optional local browser traffic can enter through the frontend helper `127.0.0.1:8081`, which is served by the same server process and proxied back to the backend listener
- The estimates below focus on backend capacity; the optional helper listener overhead is intentionally excluded

## 2) Traffic Model Inputs

Define these inputs for your environment:

- `MAU`: monthly active users
- `CCU_peak`: peak concurrent online users
- `msg_rate_peak`: peak accepted messages per second platform-wide
- `p_offline`: fraction of target recipients offline at send time
- `avg_room_size`: average members per room
- `fanout_online_avg`: average online recipients per message
- `payload_bytes_avg`: average message JSON bytes
- `sync_qps_peak`: peak SYNC requests per second
- `list_qps_peak`: peak HTTP list-messages requests per second

## 3) Service-Level Throughput Envelope

For each accepted message, approximate write path:

- Postgres: 1 message row + 1 dedup row (+ outbox row only on fallback paths)
- Valkey: 1 room-stream `XADD` + 1 event-stream `XADD`

Approx baseline ops:

- Postgres inserts/s ~= `2 * msg_rate_peak` (without fallback pressure)
- Valkey writes/s ~= `2 * msg_rate_peak`
- WS broadcasts/s ~= `msg_rate_peak * fanout_online_avg`
- Push attempts/s ~= `msg_rate_peak * p_offline * (avg_room_size - 1)`

## 4) Valkey Capacity

### 4.1 Streams retention windows

Room replay stream window (per hot room):

`room_retention_sec ~= room_stream_maxlen / room_msg_rate`

Global event stream window:

`event_retention_sec ~= event_stream_maxlen / msg_rate_peak`

With defaults:

- room: `20000 / room_msg_rate`
- event: `50000 / msg_rate_peak`

### 4.2 Valkey memory envelope (rough)

`stream_mem ~= entries * (payload_bytes_avg + redis_overhead_bytes)`

Quick assumption:

- `payload_bytes_avg`: 300 to 800
- overhead: 120 to 300

Plus non-stream keys:

- presence keys ~= `online_devices`
- ack keys ~= `active_user_device_room tuples`
- room membership sets ~= `sum(room_member_count)`
- push token hashes ~= `users_with_push_tokens`

### 4.3 What happens at/after stream maxlen

- Oldest stream entries are trimmed.
- New writes usually continue.
- Slow clients can miss replay window.
- Postgres remains durable source of truth.

## 5) Postgres Capacity Envelope

### 5.1 Write load

Baseline per message:

- `message_dedup` insert attempt
- `messages` insert

So:

`postgres_write_ops_per_sec ~= 2 * msg_rate_peak (+ outbox_writes_on_failures)`

### 5.2 Read load

Major read paths:

- SYNC/List by room
- reply hydration (`GetByIDs`)
- membership checks

Approx:

`postgres_read_qps ~= sync_qps_peak * read_cost_sync + list_qps_peak * read_cost_list + fallback_reads`

Fallback reads spike when stream replay misses expected windows.

### 5.3 Storage growth

If each message row footprint (including indexes) is `msg_row_bytes_eff`:

`daily_message_storage ~= msg_rate_avg * 86400 * msg_row_bytes_eff`

Do this separately for:

- `messages`
- `message_dedup`
- `message_outbox` (usually small if relay healthy)

## 6) WebSocket Capacity Envelope

### 6.1 Connection load

- Active connections ~= online users * devices per user.
- Memory per connection depends on goroutine stack, socket buffers, and outbound queue.

Approx:

`ws_mem ~= active_connections * mem_per_conn`

### 6.2 Broadcast pressure

`frames_per_sec ~= msg_rate_peak * fanout_online_avg`

If consumer is slow:

- outbound queue fills
- server currently drops frame and disconnects client (forcing resync path)

## 7) Outbox/Fallback Envelope

Outbox accumulates when stream append or publish fails.

Backlog growth:

`outbox_growth_per_sec ~= fallback_enqueues_per_sec - relay_drain_per_sec`

Current relay ceiling (simplified):

- max claim batch: 200 every 2s
- theoretical upper bound ~= 100 items/s per relay worker (before retries/errors/latency effects)

If failures produce >100 pending/s continuously, backlog grows.

## 8) Push Notification Envelope

Push attempts per second:

`push_qps ~= msg_rate_peak * p_offline * (avg_room_size - 1)`

Timeout budget:

- current per-call timeout: 2s

If provider latency > timeout or error rate rises, retries are currently best-effort per message path; monitor separately from message durability.

## 9) Worked Example (Illustrative)

Assume:

- `msg_rate_peak = 200 msg/s`
- `fanout_online_avg = 4`
- `avg_room_size = 6`
- `p_offline = 0.35`
- `payload_bytes_avg = 500`

Then:

- Valkey writes/s ~= `400`
- Postgres baseline inserts/s ~= `400`
- WS frames/s ~= `800`
- Push attempts/s ~= `200 * 0.35 * 5 = 350`
- Global event replay window with default 50k ~= `50000/200 = 250s` (~4.2 min)
- Busy room at 20 msg/s with default 20k window ~= `1000s` (~16.7 min)

Interpretation: at this load, replay windows are short; if product expects longer reconnect tolerance, increase maxlen and Valkey memory budget.

## 10) Sizing Process (Recommended)

1. Pick product SLOs:
   reconnect tolerance, acceptable missed realtime window, max push delay.
2. Convert SLOs to capacity numbers:
   required stream maxlen, required outbox drain rate, required DB write/read headroom.
3. Apply burst factor:
   2x to 3x over steady-state peak.
4. Validate with load test:
   verify p95/p99 latency, queue growth, stream trim behavior, relay recovery time.
5. Set alerts on leading indicators:
   Valkey memory %, stream window age, outbox backlog, publish/append errors, DB write latency.

## 11) Business Safety Notes

- Postgres is the durable source of truth.
- Valkey streams are fast replay buffers with bounded retention.
- If business requires guaranteed long offline catch-up, add explicit timeline sync from Postgres (cursor by `created_at/message_id`) as first-class path, not only stream replay.
