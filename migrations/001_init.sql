CREATE TABLE IF NOT EXISTS settings (id integer PRIMARY KEY CHECK (id=1), sending_paused boolean NOT NULL DEFAULT true);
INSERT INTO settings(id) VALUES(1) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS outbound_jobs (
 id bigserial PRIMARY KEY, location_id text NOT NULL, message_id text NOT NULL, to_number text NOT NULL,
 from_number text NOT NULL, body text NOT NULL, payload jsonb NOT NULL, status text NOT NULL DEFAULT 'queued',
 attempts integer NOT NULL DEFAULT 0, available_at timestamptz NOT NULL DEFAULT now(), provider_message_id text,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(location_id,message_id)
);
CREATE INDEX IF NOT EXISTS outbound_ready_idx ON outbound_jobs(status,available_at);
CREATE TABLE IF NOT EXISTS suppressions (phone_number text PRIMARY KEY, source text NOT NULL, provider_event_id text, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS delivery_events (provider_event_id text PRIMARY KEY, job_id bigint REFERENCES outbound_jobs(id), status text NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS inbound_messages (provider_event_id text PRIMARY KEY, from_number text NOT NULL, to_number text NOT NULL, body text NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
