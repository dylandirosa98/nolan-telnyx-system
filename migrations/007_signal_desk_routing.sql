ALTER TABLE inbound_messages
ADD COLUMN IF NOT EXISTS signal_desk_resolved_at timestamptz;
