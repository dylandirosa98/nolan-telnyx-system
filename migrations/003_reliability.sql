ALTER TABLE outbound_jobs
 ADD COLUMN IF NOT EXISTS locked_until timestamptz;

CREATE INDEX IF NOT EXISTS outbound_reclaim_idx
 ON outbound_jobs(status, locked_until)
 WHERE status='sending';

ALTER TABLE inbound_messages
 ADD COLUMN IF NOT EXISTS processed_at timestamptz;
