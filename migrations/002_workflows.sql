CREATE TABLE IF NOT EXISTS workflow_enrollments (
 id bigserial PRIMARY KEY,
 external_id text NOT NULL,
 location_id text NOT NULL,
 workflow_key text NOT NULL,
 contact_id text NOT NULL,
 to_number text NOT NULL DEFAULT '',
 from_number text NOT NULL DEFAULT '',
	contact_timezone text NOT NULL DEFAULT '',
 variables jsonb NOT NULL DEFAULT '{}'::jsonb,
 consent_at timestamptz,
 consent_source text NOT NULL DEFAULT '',
 state text NOT NULL DEFAULT 'pending',
 next_run_at timestamptz,
 variant integer NOT NULL DEFAULT 0,
 sent_count integer NOT NULL DEFAULT 0,
 locked_until timestamptz,
 last_reply text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(location_id, external_id)
);
CREATE INDEX IF NOT EXISTS workflow_enrollments_ready_idx
 ON workflow_enrollments(state, next_run_at)
 WHERE state IN ('pending', 'awaiting_reply');
CREATE INDEX IF NOT EXISTS workflow_enrollments_phone_idx
 ON workflow_enrollments(to_number, state);

ALTER TABLE outbound_jobs
 ADD COLUMN IF NOT EXISTS workflow_enrollment_id bigint REFERENCES workflow_enrollments(id);

CREATE TABLE IF NOT EXISTS crm_jobs (
 id bigserial PRIMARY KEY,
 workflow_enrollment_id bigint NOT NULL REFERENCES workflow_enrollments(id),
 action text NOT NULL,
 body text NOT NULL DEFAULT '',
 payload jsonb NOT NULL DEFAULT '{}'::jsonb,
 status text NOT NULL DEFAULT 'queued',
 attempts integer NOT NULL DEFAULT 0,
 available_at timestamptz NOT NULL DEFAULT now(),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(workflow_enrollment_id, action)
);
CREATE INDEX IF NOT EXISTS crm_jobs_ready_idx ON crm_jobs(status, available_at);

CREATE TABLE IF NOT EXISTS workflow_rate_limits (
 workflow_key text PRIMARY KEY,
 next_allowed_at timestamptz NOT NULL DEFAULT now(),
 batch_remaining integer NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
