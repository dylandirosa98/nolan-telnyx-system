# Messaging Compliance Design

**Status:** Draft — Telnyx API access is verified. The brand is accepted and the TCR campaign status is ACTIVE, but the Telnyx campaign review is `TELNYX_FAILED` pending opt-in and privacy-policy corrections. No sending number is assigned.
**Purpose:** Define technical safeguards. This document is not legal advice.

## Responsibility split

The external postcard system acquires the homeowner's initial consent and produces a CSV of completed opt-ins. The client is responsible for the accuracy and permitted use of imported contacts. This integration is responsible for preserving sending eligibility and processing revocation after contacts enter HighLevel.

## Opt-out design goal

When a recipient revokes consent, the system must stop future promotional SMS promptly, including messages already queued but not yet submitted to the provider. A revocation must remain effective across campaigns and application restarts.

## Defense-in-depth model

Opt-out enforcement must not depend on one database or platform alone.

1. **Messaging provider block**
   - Keep the provider's standard STOP filtering enabled.
   - Use the provider's recognized opt-out keywords and approved confirmation behavior.
   - Do not send a second application-generated confirmation when the provider already sends one.

2. **Integration suppression record**
   - Receive the provider's signed inbound-message webhook.
   - Deduplicate the event.
   - Persist a durable suppression record containing the normalized destination number, scope, source, provider event ID, and timestamp.
   - Treat suppression as authoritative before submitting any later outbound message.

3. **Queued-work cancellation**
   - Mark unsent jobs for the suppressed number as cancelled.
   - A worker must re-check suppression immediately before every provider API call; checking only when the job is first queued is insufficient.

4. **HighLevel synchronization**
   - Add the inbound message to the HighLevel conversation.
   - Set the contact's SMS DND state through the supported HighLevel API or agreed workflow.
   - Preserve the original inbound text for staff context, subject to the project's data-retention policy.

5. **Auditability**
   - Record the webhook receipt, suppression decision, queue cancellations, HighLevel synchronization result, and any retry/failure.
   - Never log provider credentials or unnecessary contact data.

## Recognized revocations

At minimum, support the provider's standard keywords case-insensitively and after trimming surrounding whitespace. Provider defaults currently include common terms such as:

- STOP
- STOPALL / STOP ALL
- UNSUBSCRIBE
- CANCEL
- END
- QUIT

Provider keyword handling is necessary but may not cover every reasonable natural-language request. Messages such as "please don't text me" or "remove me" must remain visible to staff and have a reliable manual SMS-DND action. Before launch, decide whether the MVP will add conservative phrase matching or rely on immediate staff handling for non-keyword requests.

## Re-subscription

Re-subscription must not occur merely because a CSV is imported again. The provider and local suppression state must agree before sending resumes. The exact START/UNSTOP workflow and required evidence will be defined after selecting the provider and reviewing the approved campaign.

## Failure behavior

- If local suppression status cannot be checked, fail closed and do not send.
- If HighLevel DND synchronization fails, keep the local and provider blocks active and retry the synchronization.
- If a provider rejects a send because of its STOP block, create or refresh the local suppression record and cancel remaining queued work.
- Duplicate or out-of-order webhooks must not undo a newer suppression.
- Administrative overrides must be restricted, authenticated, and audited; ordinary users must not bypass suppression.

## Provider notes

### Telnyx

Telnyx documents automatic block rules for standard opt-out keywords at the messaging-profile level and emits an inbound webhook with an `autoresponse_type` value such as `STOP`. A blocked send returns error code `40300`. The integration should consume that webhook and mirror the block locally and into HighLevel.

### Twilio

Twilio documents built-in handling for standard opt-out keywords. For Messaging Services, opt-out state can exist at both the service and sender level; blocked sends can return error `21610`. The integration should preserve Twilio's built-in filtering and synchronize its result locally and into HighLevel.

## Verification scenarios

Before production launch, automated and end-to-end tests must prove:

1. A standard STOP reply creates provider, local, and HighLevel suppression.
2. STOP cancels all unsent jobs for that number.
3. A worker cannot send a job queued before STOP but processed afterward.
4. Duplicate STOP webhooks are harmless.
5. Out-of-order delivery events cannot re-enable the contact.
6. Provider-block errors create/refresh local suppression.
7. A non-keyword revocation is visible and can be placed on DND reliably.
8. A suppressed contact reappearing in a later CSV remains suppressed.
9. Re-subscription requires the approved explicit workflow.
10. No duplicate opt-out confirmation is sent.
