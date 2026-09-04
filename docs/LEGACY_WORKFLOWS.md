# Legacy HighLevel workflow reference

## Purpose and verification status

This document records the 17 workflows inspected read-only in the legacy HighLevel location on 2026-09-04. The API export returned 17 unique workflow definitions with no read failures: 12 were published and 5 were drafts.

This is an implementation reference, not launch approval. The export contained action graphs but not enrollment trigger definitions. All code equivalents therefore remain disabled until their trigger, destination CRM mappings, approved message copy, sender, consent source, and 10DLC campaign are verified.

The ignored local evidence is stored in `.local/legacy-workflows.raw.json` and `.local/ghl-source-templates.json`. It is not committed because it contains client-specific message copy and legacy identifiers.

## Safe code translation

The implementation normalizes the workflows into three code families:

- `RecurringFollowUp`: durable wait, one approved SMS, explicit reply handling, bounded recurrence, recipient-local send windows, suppression checks, and an idempotent CRM action.
- `ManualFollowUp`: durable wait, create one assigned task, then perform one idempotent opportunity transition.
- `SMSBlast`: one approved message, explicit rate control, one enrollment per contact, and reply routing. Draft workflows and legacy randomization are quarantined.

Safety differences from the old graphs are intentional:

- STOP and equivalent carrier opt-outs suppress immediately and cancel queued work.
- “Not interested” stops the sequence and routes the conversation; it does not automatically create a global opt-out.
- Recurring sends are capped instead of looping forever.
- Message timing uses the recipient’s timezone rather than only the HighLevel account timezone.
- Random near-duplicate copy, false-persona wording, and claims such as “drove by” are not approved production content.
- A workflow cannot be enabled merely because it existed or was published in the old account.

## Recurring SMS follow-ups

### 1 x month Text

- Legacy ID: `03480f5d-09a1-4a2d-82fe-ddffb268efe9`
- Code key: `monthly-text`
- Legacy status: Published
- Legacy flow: wait 30 days, send the full-address follow-up, wait 30 days for a reply, loop to the SMS on no reply, or update the opportunity on reply.
- Legacy CRM target: pipeline `IzedLgbGlDWhFFU5kUqv`, stage `a3c42b19-6e28-446b-9e49-85321e479c3d`.
- Code behavior: 30-day interval, stop on any reply, maximum 12 sends, CRM mapping required before activation.

### 1x 3months text

- Legacy ID: `4c3d49be-1ecf-4a39-a40a-68cec8c96844`
- Code key: `three-month-text`
- Legacy status: Published
- Legacy flow: same as the monthly workflow, with 90-day waits.
- Legacy CRM target: pipeline `IzedLgbGlDWhFFU5kUqv`, stage `a3c42b19-6e28-446b-9e49-85321e479c3d`.
- Code behavior: 90-day interval, stop on any reply, maximum 12 sends, CRM mapping required.

### 1x week follow up

- Legacy ID: `aef8d440-bde4-4587-9430-c2597d6c7495`
- Code key: `weekly-follow-up`
- Legacy status: Published
- Legacy flow: wait 7 days, send the full-address follow-up, wait up to 7 days for a reply, update the opportunity on reply, or loop to another SMS on timeout.
- Legacy CRM target: pipeline `3xSixvpnkjQgHSSH85iF`, stage `a1e5cb3f-e165-4a5b-a966-a996263b2b8b`.
- Code behavior: 7-day interval, stop on any reply, maximum 12 sends, CRM mapping required.

### 6 day offer followup

- Legacy ID: `9af72b54-d08d-4804-be60-137412e8bd5b`
- Code key: `six-day-offer-follow-up`
- Legacy status: Published
- Legacy flow: wait 6 days, send a full-address follow-up, use a zero-day reply wait, update the opportunity on reply, or loop through another 6-day wait.
- Legacy CRM target: pipeline `xlnD52EMkzrDRZkUOqOs`, stage `3dc4c5dc-3589-4fb2-aca0-1e3304e2e963`.
- Code behavior: normalized to an explicit 6-day interval, stop on any reply, maximum 12 sends. The old zero-day wait is treated as ambiguous rather than copied literally.

### Every 2 Day check in

- Legacy ID: `71d7e7e0-2cdb-49c0-b1fa-ad2dd6ad73fb`
- Code key: `every-two-day-check-in`
- Legacy status: Published
- Legacy flow: wait 2 days, send an address-line check-in, use a zero-minute reply wait, remove the contact from the workflow on reply, or repeat after 2 days.
- Code behavior: explicit 2-day interval, stop on any reply, maximum 12 sends. The old zero-minute wait is not copied literally.

## Manual task follow-ups

All six legacy workflows wait for a configured period, create a task for the contact’s assigned user, create or update an opportunity in pipeline `Osdb6IdRAEil3i68QwHQ` at stage `20582696-2376-4803-93bb-d920ea49cf70`, then remove the opportunity from pipeline `3xSixvpnkjQgHSSH85iF`.

The code treats that multi-step CRM change as one idempotent operation. The legacy IDs must be mapped to the destination location before activation.

| Workflow | Legacy ID | Code key | Wait | Legacy task body | Status |
|---|---|---|---:|---|---|
| Copy - Manual Follow Up 6 Month | `1f3b250f-32d8-48f0-85ba-cd238eea1286` | `manual-follow-up-six-month` | 180 days | `6 mo follow uo` | Published |
| Manual Follow Up 1 Month | `2b23365c-f0f2-4f1e-aad9-37af5b45f7a7` | `manual-follow-up-one-month` | 30 days | `1 mo follow uo` | Published |
| Manual Follow Up 1 Week | `768da7a4-d08a-41e8-adca-985fd67fd981` | `manual-follow-up-one-week` | 7 days | `7 day follow uo` | Published |
| Manual Follow Up 2 Month | `8ddaf5f9-17c9-4213-b388-c2f36682e2d6` | `manual-follow-up-two-month` | 60 days | `2 mo follow uo` | Published |
| Manual Follow Up 2 Week | `ce2d51e4-8e12-4430-9ae8-d051e2f3174c` | `manual-follow-up-two-week` | 14 days | `14 day follow uo` | Published |
| Manual Follow Up 3 Month | `64e35e08-24bf-478a-9823-07d0ebf49cf5` | `manual-follow-up-three-month` | 90 days | `3 mo follow uo` | Published |

## SMS blast workflows

The five related blast graphs use one or more drip gates, generate a random integer from 0 through 5, but branch only for 1 through 5. If the bounds are inclusive, the unhandled 0 can silently send nothing. They then use zero-day reply waits and appointment-intent labels (`schedule-yes` and `schedule-no`) to classify property-sale responses. That behavior is not copied into production code.

### Copy - Copy - SMS Blast

- Legacy ID: `0962dcf7-bd2c-494f-8f40-39769ab0fc9`
- Code key: `copy-copy-sms-blast`
- Legacy status: Draft; cannot be enabled
- Legacy window/rate: 09:00–20:00 account time; two 1-contact-per-3-minute gates.
- Legacy content: five near-duplicate full-address variants.
- Code treatment: cataloged for traceability only and quarantined.

### SMS Blast

- Legacy ID: `5c9cbd30-617b-4cbe-8da8-fa3f0a90013d`
- Code key: `sms-blast`
- Legacy status: Draft; cannot be enabled
- Legacy window/rate: 09:00–20:00 account time; two 1-contact-per-2.25-minute gates.
- Legacy content: five near-duplicate full-address variants.
- Code treatment: cataloged for traceability only and quarantined.

### Copy - SMS Blast

- Legacy ID: `d4df54ce-f8c3-4271-a213-759efa3deb64`
- Code key: `copy-sms-blast`
- Legacy status: Draft; cannot be enabled
- Legacy window/rate: 11:30–19:00 account time; three 6-contact-per-minute gates.
- Legacy content: five address-line variants; one contains the typo `sellin`.
- Code treatment: cataloged for traceability only and quarantined.

### SMS Blast Full Address

- Legacy ID: `022f3c59-d35e-4d25-b6ea-81d1cd6f932e`
- Code key: `sms-blast-full-address`
- Legacy status: Published
- Legacy window/rate: 11:30–19:00 account time; sequential 8/minute, 8/minute, then 7/minute gates.
- All five branches reference legacy SMS snippet `hLHTsONHrgBoiP1agVNT`; the resolved text is retained only in the ignored source export.
- Negative handling enables global DND, marks the conversation read, and archives it.
- Code treatment: one canonical template, 7/minute conservative rate, recipient-local window, explicit STOP/not-interested separation, no auto-archive until policy approval.

### Upper Darby- SMS Blast

- Legacy ID: `53122bbf-5a7a-4aa6-95ad-384107ce237b`
- Code key: `upper-darby-sms-blast`
- Legacy status: Draft; cannot be enabled
- Legacy window/rate: 09:00–20:00 account time; two 10-contact-per-minute gates.
- Legacy content: five variants using different personas and potentially misleading locality/drive-by claims.
- Code treatment: cataloged for traceability only and quarantined.

### smsv2

- Legacy ID: `2a9d341f-bbd3-43b8-8b70-0815cfbf5e7b`
- Code key: `sms-v2`
- Legacy status: Draft; cannot be enabled
- Legacy flow: assigns user `sUZGtTrxsrEiLjipbmSc`, uses sequential rate gates of 1 per 1.35 minutes and 1 per 1.75 minutes, and chooses among five persona/drive-by variants.
- Its “bad response” branch includes STOP and negative replies but disables DND. Its other branch creates a lead in pipeline `X6sefRCk9DKzT8xeKQl2`, stage `e8391dab-7272-41bc-ac84-1c44ea77ed39`.
- Code treatment: permanently quarantined in its legacy form. The inverted opt-out behavior and false-positive lead creation must never be reproduced.

## Activation blockers

A workflow remains disabled until all applicable items are complete:

- The missing enrollment trigger is explicitly defined and tested.
- The destination HighLevel pipeline, stage, user, and conversation-provider mappings are configured.
- The contact has documented SMS consent with source and timestamp.
- The sender number is assigned to an approved Telnyx messaging profile and matching 10DLC campaign.
- Message copy, identity, opt-out wording, cadence, expiry, and maximum count are approved.
- STOP, duplicate events, ordinary replies, quiet hours, rate limits, retries, and worker-crash recovery pass integration tests.
- A small controlled pilot passes before wider activation.
