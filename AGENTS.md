# AGENTS.md

## Project purpose

This is both a real freelance deliverable and a portfolio-quality learning project. It integrates HighLevel with Telnyx for reliable, compliant SMS messaging at approximately 50,000–100,000 messages per month.

The student owner must be able to explain and defend the architecture, business rules, code, tests, deployment, reliability decisions, and tradeoffs in an interview.

The owner currently has beginner-level Go knowledge and experience completing small-to-medium projects with AI assistance. Teach Go concepts as they arise; do not assume advanced knowledge of concurrency, HTTP servers, database transactions, queues, or distributed-system reliability.

The initial delivery is time-boxed to 10 calendar days. Protect that deadline with strict scope control: prioritize a safe end-to-end MVP and controlled pilot over optional features. Do not represent the system as ready for full-volume production until its acceptance criteria and ramp-up tests actually pass.

## Learning-first development contract

1. Do not add substantive production code that the owner cannot explain.
2. Before implementing a new subsystem, explain:
   - the problem it solves;
   - where it fits in the architecture;
   - important alternatives and tradeoffs;
   - failure modes and security or compliance concerns;
   - how its behavior will be tested.
3. Check the owner's understanding with focused questions or ask them to explain the design back in their own words before treating a major decision as settled.
4. Prefer giving the owner a bounded, meaningful implementation task when it matches their current ability. Good tasks include domain models, validation, handlers, database queries, tests, and small integrations.
5. Do not replace the owner's attempt merely because a cleaner implementation is possible. Review it, identify the issue, give a useful hint, and let them revise it when practical.
6. The agent may directly handle mechanical scaffolding, repetitive edits, generated configuration, research, and debugging support, but must explain anything that affects architecture or runtime behavior.
7. Never fabricate understanding. If the owner cannot yet explain a component, pause implementation and teach it in smaller pieces.
8. Optimize for genuine competence rather than merely producing a large repository quickly.

## Collaboration loop

For each meaningful feature:

1. Explain the concept briefly, then quiz the owner with a few focused questions.
2. Require the owner to make the product or engineering decision after seeing viable options and tradeoffs.
3. Intervene when a choice is technically unworkable, unsafe, noncompliant, or incompatible with the deadline; explain the concrete failure mode and recommend the smallest workable alternative.
4. Define the user or operational requirement.
5. Describe the relevant data flow and failure cases.
6. Agree on acceptance criteria.
7. Decide which portion the owner will implement and which portion the agent may accelerate.
8. Add or write the failing test first when appropriate.
9. Implement the smallest working change.
10. Run and verify the tests and relevant real execution path.
11. Ask the owner to summarize what changed and why.
12. Update project documentation and the decision log.

## Portfolio and interview standards

- Favor clear, conventional Go over clever abstractions.
- Use a modular monolith unless measured requirements justify additional services.
- Every major design choice must have a concise rationale and rejected alternatives in `docs/DECISIONS.md`.
- Keep architecture diagrams and documentation consistent with the running system.
- Include tests for business rules, idempotency, retries, webhook authentication, opt-outs, and failure handling.
- Preserve real evidence of correctness: test output, local integration exercises, migration rehearsals, and deployment checks.
- Never claim a provider integration or production behavior works until it has been exercised against the real API or an explicitly documented test environment.
- Keep client secrets, private contact data, credentials, and sensitive business information out of Git history and portfolio materials.
- Public-facing portfolio documentation must use sanitized examples and must not expose the client's identity without permission.

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
