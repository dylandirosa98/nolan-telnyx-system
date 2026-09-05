ALTER TABLE inbound_messages
 ADD COLUMN IF NOT EXISTS attempts integer NOT NULL DEFAULT 0,
 ADD COLUMN IF NOT EXISTS failed_at timestamptz;

ALTER TABLE delivery_events
 ADD COLUMN IF NOT EXISTS attempts integer NOT NULL DEFAULT 0,
 ADD COLUMN IF NOT EXISTS failed_at timestamptz;

CREATE INDEX IF NOT EXISTS inbound_retry_idx
 ON inbound_messages(processed_at, failed_at, locked_until)
 WHERE processed_at IS NULL AND failed_at IS NULL;

CREATE INDEX IF NOT EXISTS delivery_retry_idx
 ON delivery_events(highlevel_synced_at, failed_at, locked_until)
 WHERE highlevel_synced_at IS NULL AND failed_at IS NULL;
