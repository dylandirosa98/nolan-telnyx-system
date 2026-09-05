ALTER TABLE crm_jobs
 ADD COLUMN IF NOT EXISTS locked_until timestamptz;

CREATE INDEX IF NOT EXISTS crm_jobs_reclaim_idx
 ON crm_jobs(status, locked_until)
 WHERE status IN ('queued', 'running');

CREATE TABLE IF NOT EXISTS oauth_tokens (
 provider text NOT NULL,
 location_id text NOT NULL,
 access_token text NOT NULL,
 refresh_token text NOT NULL DEFAULT '',
 token_type text NOT NULL DEFAULT 'Bearer',
 user_type text NOT NULL DEFAULT '',
 scope text NOT NULL DEFAULT '',
 expires_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (provider, location_id)
);

CREATE TABLE IF NOT EXISTS admin_audit (
 id bigserial PRIMARY KEY,
 action text NOT NULL,
 detail jsonb NOT NULL DEFAULT '{}'::jsonb,
 created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE inbound_messages
 ADD COLUMN IF NOT EXISTS locked_until timestamptz;

ALTER TABLE delivery_events
 ADD COLUMN IF NOT EXISTS provider_message_id text;

CREATE TABLE IF NOT EXISTS oauth_states (
 state text PRIMARY KEY,
 created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL
);

ALTER TABLE settings
 ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();
