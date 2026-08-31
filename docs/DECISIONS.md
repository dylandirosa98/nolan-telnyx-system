# Decision log

## 2026-08-31 — Telnyx-only local MVP

The requested MVP uses Telnyx only. Twilio and other providers are excluded to protect the ten-day scope. A small provider interface exists for fake tests and future replacement, not as a premature multi-provider system.

## 2026-08-31 — PostgreSQL queue

PostgreSQL provides durable state and transactional row claiming without adding Redis/Kafka. `SKIP LOCKED` permits multiple workers to claim distinct jobs. The tradeoff is that queue throughput must be measured before the stated monthly scale is accepted.

## 2026-08-31 — Unverified HighLevel boundary

HighLevel signature/payload/DND contracts were not provided and no account exists. Interfaces and configurable bearer authentication prevent fabricated claims; authoritative API documentation and live testing are required before launch.
