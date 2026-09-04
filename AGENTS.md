# AGENTS.md

## Project purpose

This is a time-boxed freelance deliverable integrating HighLevel with Telnyx for reliable, compliant SMS messaging at approximately 50,000–100,000 messages per month.

The initial delivery is time-boxed to 10 calendar days. Protect that deadline with strict scope control: prioritize a safe end-to-end MVP and controlled pilot over optional features. Do not represent the system as ready for full-volume production until its acceptance criteria and ramp-up tests actually pass.

## Delivery-first development contract

1. Prioritize a working, verified client deliverable over teaching, quizzes, interview preparation, or preserving implementation tasks for the owner.
2. The agent may directly design and implement production code, tests, scaffolding, configuration, documentation, deployment, and debugging.
3. Make necessary engineering decisions using the smallest safe approach compatible with the deadline; ask the owner only when a product requirement, external credential, legal approval, or irreversible choice genuinely requires their input.
4. Explain architecture or code only when the owner asks, when a decision needs approval, or when a material risk must be understood.
5. Use conventional Go and avoid unnecessary abstractions, services, dependencies, and speculative features.
6. Add or update tests for important behavior and verify real execution paths before claiming completion.
7. Keep documentation aligned with the running system and clearly distinguish implemented, tested, provisional, and blocked behavior.

## Engineering standards

- Use a modular monolith unless measured requirements justify additional services.
- Every major design choice must have a concise rationale and rejected alternatives in `docs/DECISIONS.md`.
- Keep architecture diagrams and documentation consistent with the running system.
- Include tests for business rules, idempotency, retries, webhook authentication, opt-outs, and failure handling.
- Preserve real evidence of correctness: test output, local integration exercises, migration rehearsals, and deployment checks.
- Never claim a provider integration or production behavior works until it has been exercised against the real API or an explicitly documented test environment.
- Keep client secrets, private contact data, credentials, and sensitive business information out of Git history and documentation.
- Use sanitized examples and do not expose the client's identity without permission.

## Current architectural direction

This direction is provisional until business requirements are defined:

- Go modular monolith.
- Railway deployment.
- Separate API and worker processes built from one repository.
- PostgreSQL for durable application state and background jobs.
- HighLevel remains the operator-facing CRM and campaign interface.
- A HighLevel replacement SMS conversation provider forwards outbound work to the service.
- Telnyx provides SMS transport and sends inbound-message and delivery-status webhooks.
- Webhook processing must be authenticated, durable, idempotent, and asynchronous.
- The design is location-aware even if the first deployment serves only one HighLevel sub-account.

## Documentation expectations

Maintain these living documents as the project develops:

- `README.md` — purpose, status, setup, and demonstration instructions.
- `docs/REQUIREMENTS.md` — functional and non-functional requirements.
- `docs/ARCHITECTURE.md` — components, data flows, boundaries, and failure handling.
- `docs/COMPLIANCE.md` — consent, registration, opt-out, suppression, and sending rules.
- `docs/MIGRATION.md` — HighLevel transfer, provider cutover, rollback, and verification.
- `docs/PLAN.md` — phases, milestones, risks, and acceptance criteria.
- `docs/DECISIONS.md` — dated architectural decision records and unresolved questions.

Do not present assumptions as settled requirements. Label them explicitly and replace them when the client provides authoritative information.
