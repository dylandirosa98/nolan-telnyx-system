# MVP plan and acceptance

Completed locally: domain validation/keywords/retry classification, Telnyx signature verification, PostgreSQL schema, idempotent enqueue, concurrent claim query, suppression cancellation, API/worker commands, container packaging, and fake provider seams.

## Verification evidence (2026-08-31)

- The initial domain test was intentionally red because `ValidateE164`, `IsOptOut`, and retry classification were absent; after the smallest implementation, it passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `go test -race ./...` passed.
- `DATABASE_URL=postgres://messaging:messaging@localhost:55432/messaging go test ./internal/app -count=1` passed against Compose PostgreSQL, including workflow E2E, CRM worker E2E, and admin status.

Before pilot: purchase one SMS-capable pilot number, assign it to the active 10DLC campaign and messaging profile, install the HighLevel Conversation Provider app, complete OAuth, and confirm webhook payloads/signatures, DND and conversation APIs, delivery correlation, consent evidence, quiet hours, monitoring, and rollback. Full-volume production is not accepted until ramp tests pass.
