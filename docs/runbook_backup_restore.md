# Backup and Restore Runbook

## Scope

Covers Postgres (room metadata + messages + outbox) and Valkey (realtime state cache/streams). The optional frontend helper is a stateless asset bundle plus local listener inside the server image/process and is not part of backup scope.

## Postgres

Backup:
- Run logical backup daily:
  - `pg_dump -Fc -h <host> -U <user> -d aelora > aelora_$(date +%F).dump`
- Store in versioned object storage.

Restore:
- `createdb aelora_restore`
- `pg_restore -h <host> -U <user> -d aelora_restore aelora_YYYY-MM-DD.dump`
- Validate row counts for `rooms`, `room_members`, `messages`, and `message_outbox`.

## Valkey

Backup:
- Enable AOF + RDB persistence.
- Copy RDB/AOF snapshots to object storage at regular intervals.

Restore:
- Start replacement Valkey with persisted files.
- Verify stream keys and ACK/presence keys are readable.

## Drill Policy

- Run restore drill at least monthly.
- Track RPO and RTO from drill results.
- Keep runbook and credentials tested and current.
- Restore the matching application image so the optional frontend helper assets stay aligned with the backend API.
