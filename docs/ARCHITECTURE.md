# Architecture

The API and worker are separate commands in one conventional Go modular monolith. PostgreSQL is the durable source for outbound jobs, inbound events, suppressions, delivery events, workflow enrollments, CRM jobs, OAuth tokens, and the operator sending-pause setting.

HighLevel outbound webhook → authentication seam → location bind → validation → unique `(location_id,message_id)` enqueue → worker claim with `SKIP LOCKED` → suppression recheck → Telnyx adapter.

Telnyx inbound webhook → timestamp + Ed25519 verification over `{timestamp}|{json}` → persist event → HTTP 202 → asynchronous HighLevel forwarding, workflow replies, and STOP/DND sync. Duplicate events are ignored. STOP is applied only on inbound `message.received` (including provider `autoresponse_type=STOP`). Inbound and delivery synchronization use bounded exponential retries and become operator-visible dead letters after 5 failed background attempts.

Workflow engine commands enqueue SMS jobs and CRM jobs in the same transaction. The CRM worker executes HighLevel create-task, contact-note, and conversation-archive calls with retries. Opportunity-stage IDs are not hard-coded; CRM actions other than `create_task` and `archive_conversation` are recorded as contact notes until destination pipeline IDs are confirmed.

The `ENABLE_SENDING` environment flag is a process-level safety gate; `settings.sending_paused` is the operator kill switch (`POST /admin/sending`). Provider HTTP calls have a 15-second client timeout. HighLevel API version header is `2021-07-28`. Logs do not include bodies or secrets.

OAuth access/refresh tokens are stored in PostgreSQL for the installed location and refreshed on expiry or HTTP 401. Tokens are not encrypted at rest; restrict database access and rotate credentials after the pilot.
