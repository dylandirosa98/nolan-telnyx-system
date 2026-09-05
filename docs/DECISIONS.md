# Decision log

## 2026-08-31 — Telnyx-only local MVP

The requested MVP uses Telnyx only. Twilio and other providers are excluded to protect the ten-day scope. A small provider interface exists for fake tests and future replacement, not as a premature multi-provider system.

## 2026-08-31 — PostgreSQL queue

PostgreSQL provides durable state and transactional row claiming without adding Redis/Kafka. `SKIP LOCKED` permits multiple workers to claim distinct jobs. The tradeoff is that queue throughput must be measured before the stated monthly scale is accepted.

## 2026-08-31 — Unverified HighLevel boundary

HighLevel signature/payload/DND contracts were not provided and no account exists. Interfaces and configurable bearer authentication prevent fabricated claims; authoritative API documentation and live testing are required before launch.

## 2026-09-04 — Telnyx webhook signed payload uses a pipe

Telnyx signs `{timestamp}|{json_payload}`. The local verifier previously used a dot separator, which would reject every live webhook. Tests now use the documented pipe form.

## 2026-09-04 — CRM jobs execute as HighLevel tasks and notes

Queued `crm_jobs` were never claimed. The worker now executes them. `create_task` calls `POST /contacts/:id/tasks`. `archive_conversation` updates the conversation and writes a note. Other CRM actions write a contact note because destination opportunity pipeline/stage IDs are not confirmed. Rejected: hard-coding opportunity stages.

## 2026-09-04 — OAuth tokens in PostgreSQL

HighLevel location tokens are stored and refreshed on expiry or HTTP 401. Encryption at rest was rejected for the MVP to avoid a new key-management path before the pilot; database access must stay private.

## 2026-09-04 — Versioned SQL migrations

`schema_migrations` records applied files so later statements are not re-executed. Existing `IF NOT EXISTS` files remain idempotent for the first boot after this change.
