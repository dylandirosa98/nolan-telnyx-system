ALTER TABLE delivery_events
 ADD COLUMN IF NOT EXISTS highlevel_synced_at timestamptz,
 ADD COLUMN IF NOT EXISTS locked_until timestamptz;
