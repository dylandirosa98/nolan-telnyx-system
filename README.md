# HighLevel → Telnyx messaging MVP

Local, consent-oriented Telnyx-only MVP. It accepts authenticated HighLevel outbound requests, durably queues them in PostgreSQL, and has a separate worker process. Telnyx inbound webhooks are verified with the official Ed25519 `{timestamp}|{payload}` scheme, persisted, then processed asynchronously. HighLevel CRM side effects (tasks, notes, conversation archive) are queued in `crm_jobs` and executed by the worker.

Sending remains disabled by default and in Compose. Do not enable live SMS until Telnyx campaign review, a purchased number, Conversation Provider installation, and a controlled pilot succeed.

## Operator workflow: CSV import then SMS

Staff keep using HighLevel. Upload the opted-in contact CSV in HighLevel (**Contacts → Import**), then text from Conversations, bulk actions, or workflows. Those SMS requests hit this service and go out through Telnyx. Replies show up in HighLevel and the LeadConnector mobile app.

Do not upload CSVs to this service. Contact import, consent records, and campaign selection stay in HighLevel.

## Local demonstration

Install Go 1.25 or newer, or use the pinned Go 1.26.8 build image in the Dockerfile. With Docker Compose v2 unavailable, the tested command is `docker-compose up --build`. Health is `GET /healthz`; readiness is `GET /readyz`. Authenticated operator status is `GET /admin/status` with `Authorization: Bearer ***

Commands: `make fmt`, `make test`, `make vet`, `make race`, `make build`.

Set `DATABASE_URL` to run the Compose-backed end-to-end tests.

## Implemented

- Durable outbound queue, suppression, retries, quiet hours, and workflow catalog (disabled until explicitly enabled).
- Telnyx send adapter, signed webhooks, inbound forwarding, STOP/DND sync.
- HighLevel Conversation Provider outbound webhook, inbound/status adapters, OAuth token storage/refresh, and CRM job execution.
- Authenticated admin status and sending-pause control. Process flag `ENABLE_SENDING` still has to be true for any SMS to leave the worker.

## Still blocked for live traffic

- HighLevel Conversation Provider app install and provider ID.
- Working HighLevel location token (previous credential returned 403).
- Telnyx campaign `TELNYX_FAILED` corrections, purchased number, and number assignment.
- Explicit send approval and a controlled live pilot.

Start installation through the authenticated `/oauth/highlevel/start` route. It creates a single-use OAuth state before redirecting to HighLevel; the callback rejects missing, expired, or reused states.
