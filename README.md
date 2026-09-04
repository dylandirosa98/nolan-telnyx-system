# HighLevel → Telnyx messaging MVP

Local, consent-oriented Telnyx-only MVP. It accepts authenticated HighLevel outbound requests, durably queues them in PostgreSQL, and has a separate worker process. Telnyx inbound webhooks are verified with the official Ed25519 header scheme, persisted through suppression/queue cancellation, and forwarded through an unverified HighLevel adapter seam.

Live HighLevel source/destination API access and Telnyx account API access have now been verified. The Telnyx brand is accepted and the TCR campaign status is `ACTIVE`, but Telnyx reports the campaign as `TELNYX_FAILED` pending opt-in evidence and privacy-policy corrections. There is no messaging number or number-to-campaign assignment. Sending remains disabled by default and in Compose. The legacy CRM's API-addressable pipeline, field, tag, and custom-value structure has been migrated to the new HighLevel location without querying or copying CRM records. The repository does not yet claim live messaging readiness.

## Local demonstration

Install Go or use the included `golang:1.23-alpine` image. With Docker Compose v2 unavailable, the tested command is `docker-compose up --build`. Health is `GET /healthz`; readiness is `GET /readyz`. The migration creates PostgreSQL state and jobs. Supply only test credentials if you later exercise an HTTP stub; never place secrets in this repository.

Commands: `make fmt`, `make test`, `make vet`, `make race`, `make build`.

## Current limitations

HighLevel signature format, exact payload contract, DND endpoint, campaign eligibility metadata, quiet hours, and delivery-to-job correlation are intentionally interfaces/configuration seams pending authoritative client/provider documentation. A production pilot also requires approved 10DLC registration, real account/API testing, consent evidence, and operational ramp criteria.
