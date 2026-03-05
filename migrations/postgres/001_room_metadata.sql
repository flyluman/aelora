CREATE TABLE IF NOT EXISTS rooms (
  id TEXT PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS room_members (
  room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'member',
  joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (room_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_room_members_user_id ON room_members(user_id);

CREATE TABLE IF NOT EXISTS messages (
  room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL,
  message_id TEXT NOT NULL,
  sender_id TEXT NOT NULL,
  content TEXT NOT NULL,
  message_type TEXT NOT NULL DEFAULT 'text',
  client_msg_id TEXT NOT NULL,
  reply_to_message_id TEXT,
  PRIMARY KEY (room_id, created_at, message_id)
);

CREATE TABLE IF NOT EXISTS message_dedup (
  client_msg_id TEXT PRIMARY KEY,
  room_id TEXT NOT NULL,
  message_id TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS message_outbox (
  message_id TEXT PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL,
  payload JSONB NOT NULL,
  dispatched BOOLEAN NOT NULL DEFAULT FALSE,
  claimed_by TEXT,
  claimed_until TIMESTAMPTZ,
  dispatched_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_messages_room_created_at ON messages(room_id, created_at);
CREATE INDEX IF NOT EXISTS idx_messages_room_reply_to ON messages(room_id, reply_to_message_id);
CREATE INDEX IF NOT EXISTS idx_message_outbox_pending ON message_outbox(dispatched, created_at);
CREATE INDEX IF NOT EXISTS idx_message_outbox_claim ON message_outbox(dispatched, claimed_until, created_at);
