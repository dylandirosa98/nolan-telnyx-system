# Architecture

The API and worker are separate commands in one conventional Go modular monolith. PostgreSQL is the durable source for outbound jobs, inbound events, suppressions, delivery events, and the operator sending-pause setting.

HighLevel outbound webhook → authentication seam → strict validation → unique `(location_id,message_id)` enqueue → worker claim with `SKIP LOCKED` → suppression recheck → Telnyx adapter. Telnyx inbound webhook → timestamp + Ed25519 verification → idempotent event/suppression handling → queued-job cancellation and HighLevel forwarding seam.

The `ENABLE_SENDING` environment flag is a process-level safety gate; the database `settings.sending_paused` flag is the intended operator kill switch. Provider HTTP calls have a 15-second client timeout. Logs do not include bodies or secrets.
