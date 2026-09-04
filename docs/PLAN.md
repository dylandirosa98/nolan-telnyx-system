# MVP plan and acceptance

Completed locally: domain validation/keywords/retry classification, Telnyx signature verification, PostgreSQL schema, idempotent enqueue, concurrent claim query, suppression cancellation, API/worker commands, container packaging, and fake provider seams.

## Verification evidence (2026-08-31)

- The initial domain test was intentionally red because `ValidateE164`, `IsOptOut`, and retry classification were absent; after the smallest implementation, it passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `go test -race ./...` passed.
- `DATABASE_URL=postgres://messaging:messaging@localhost:55432/messaging go test ./internal/app -run TestFakeLocalEndToEnd -count=1` passed against Compose PostgreSQL and the fake Telnyx provider.
- `docker-compose up -d --build` completed; `GET /healthz` returned `{"status":"ok"}` and `GET /readyz` returned `{"status":"ready"}`.

Before pilot: purchase one SMS-capable pilot number, assign it to the active 10DLC campaign and messaging profile, exercise the live HighLevel installation and Telnyx account, and confirm webhook payloads/signatures, DND and conversation APIs, delivery correlation, consent evidence, quiet hours, monitoring, and rollback. Full-volume production is not accepted until ramp tests pass.
