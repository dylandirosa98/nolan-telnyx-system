# HighLevel → Telnyx messaging MVP

Local, consent-oriented Telnyx-only MVP. It accepts authenticated HighLevel outbound requests, durably queues them in PostgreSQL, and has a separate worker process. Telnyx inbound webhooks are verified with the official Ed25519 `{timestamp}|{payload}` scheme, persisted, then processed asynchronously. HighLevel CRM side effects (tasks, notes, conversation archive) are queued in `crm_jobs` and executed by the worker.

Sending remains disabled by default and in Compose. Do not enable live SMS until Telnyx campaign review, a purchased number, Conversation Provider installation, and a controlled pilot succeed.

## Operator workflow: CSV import then SMS

Staff can text from the operator inbox at `/inbox` while HighLevel Conversation Provider install is blocked. Import opted-in contacts in HighLevel if you still use it as a CRM; do not upload CSVs to this service. Replies are stored locally from Telnyx webhooks.

Do not upload CSVs to this service. Contact import, consent records, and campaign selection stay in HighLevel.

## Local demonstration

Install Go 1.25 or newer, or use the pinned Go 1.26.8 build image in the Dockerfile. With Docker Compose v2 unavailable, the tested command is `docker-compose up --build`. Health is `GET /healthz`; readiness is `GET /readyz`. Operator inbox is `http://localhost:8088/inbox` (no login; keys stay in env). Authenticated operator status is `GET /admin/status` with `Authorization: Bearer $ADMIN_TOKEN`.

Commands: `make fmt`, `make test`, `make vet`, `make race`, `make build`.

Set `DATABASE_URL` to run the Compose-backed end-to-end tests.

## Implemented

- Durable outbound queue, suppression, retries, quiet hours, and workflow catalog (disabled until explicitly enabled).
- Telnyx send adapter, signed webhooks, inbound forwarding, STOP/DND sync.
- HighLevel Conversation Provider outbound webhook, inbound/status adapters, OAuth token storage/refresh, and CRM job execution.
- Authenticated admin status and sending-pause control. Process flag `ENABLE_SENDING` still has to be true for any SMS to leave the worker.
- Mobile operator inbox at `/inbox` that queues SMS through the same outbound table. HighLevel Conversations is not required.

## Still blocked for live traffic

- HighLevel Conversation Provider app install and provider ID.
- Working HighLevel location token (previous credential returned 403).
- Telnyx campaign `TELNYX_FAILED` corrections, purchased number, and number assignment.
- Explicit send approval and a controlled live pilot.

Start installation through the authenticated `/oauth/start` route. It creates a single-use OAuth state before redirecting; the callback rejects missing, expired, or reused states. Marketplace redirect and delivery URLs must not contain HighLevel brand strings (White-Label listing rule).
