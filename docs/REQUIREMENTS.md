# Requirements Discovery

**Status:** Draft — discovery in progress  
**Important:** Items marked *unverified* are not approved requirements and must not be treated as facts.

## Business context

- The client is a real-estate wholesaling business.
- The intended recipients are U.S. homeowners.
- The intended messages ask whether a homeowner is interested in an offer for their property.
- The messages are promotional or marketing communications.
- HighLevel is the CRM and intended operator interface.
- The current HighLevel sub-account is controlled under another agency and is expected to be transferred to the client's own account.

## Initial scale

- Expected current volume: approximately 50,000–100,000 SMS messages per month.
- Future volume may be higher.
- A potential campaign could contain up to 500,000 recipients, although the current monthly cap would prevent sending that entire campaign in one month.
- Multiple campaigns may be active concurrently.
- No campaign-completion-time requirement has been established.

## Intended messaging capabilities

The client wants to continue operating primarily inside HighLevel. For the MVP, CSV files should be imported directly through HighLevel unless testing shows that HighLevel cannot preserve the required consent metadata or workflow. A separate upload interface is out of scope by default.

The proposed provider integration is expected to support:

- contacts imported through HighLevel's existing CSV import;
- outbound messages initiated from HighLevel;
- HighLevel workflows and bulk actions;
- inbound homeowner replies in HighLevel conversations;
- delivery and failure status handling;
- opt-out and suppression enforcement;
- safe queueing, retries, deduplication, and throughput controls.

These capabilities remain provisional until the existing workflow is inventoried.

## Consent and list provenance — clarification required

The earlier description of a "scraped homeowner list" was a misunderstanding. The currently described acquisition flow is:

1. A homeowner receives a postcard produced through an outside postcard company.
2. The postcard invites the homeowner to text a dedicated number operated by that outside system.
3. The outside system asks the homeowner to opt in.
4. The homeowner replies affirmatively (described as replying "YES").
5. The outside system adds completed opt-ins to a CSV.
6. The client imports that CSV for later messaging campaigns.

The postcard number and opt-in system are separate from HighLevel and the new provider integration. The developer does not currently have or require access to that number. The new integration begins at CSV import and does not need to reproduce the postcard opt-in workflow.

If verified, this is materially different from cold-texting scraped phone numbers and provides a plausible consent-based campaign flow. The exact implementation still needs to be documented before provider registration and production launch.

The following details remain unknown:

- the postcard wording and keyword/number;
- the exact automated opt-in disclosure;
- the affirmative response required from the homeowner;
- whether the client's business is clearly identified;
- whether the disclosure explains the message subject and expected recurrence/frequency;
- how STOP and HELP are presented;
- whether terms and privacy information are provided where required;
- which system receives the inbound message and produces the CSV;
- what consent evidence each CSV row or source system retains;
- whether any non-opted-in sources are mixed into the same files.

The production system must preserve the distinction between an inbound inquiry and completed campaign consent. Merely texting the postcard number must not automatically be treated as agreement to recurring promotional messages if the flow requires a second affirmative action.

## Provider status

- Telnyx was initially proposed.
- Twilio was previously used through LeadConnector and is also under consideration.
- Provider selection is not final.
- Both providers require consent-based messaging and prohibit unsolicited bulk traffic.
- Historical message delivery does not establish current provider-policy or legal compliance.

## Responsibility boundary

The client is the campaign owner and is responsible for:

- the source and lawfulness of uploaded contacts;
- obtaining and retaining required consent;
- providing truthful A2P registration information;
- approving campaign descriptions, opt-in language, and message content;
- obtaining legal advice for its outreach program when needed.

The developer is responsible for:

- accurately implementing the provider integration;
- not submitting invented or misleading registration information;
- enforcing the agreed sending-eligibility, opt-out, suppression, security, and audit behavior;
- documenting what information the client must supply;
- warning the client when a stated use case conflicts with provider requirements.

The developer is not expected to investigate or certify every contact personally. Before production launch, the client must provide and approve the information needed to configure A2P registration and the system's sending rules. Contractual allocation of legal liability should be reviewed by a qualified attorney.

## Compliance-by-design requirements

The intended and documented purpose of the system is consent-based, lawful business messaging. A disclaimer alone is not a safeguard; the implementation should make misuse harder and leave an auditable record.

The MVP should:

- contain no contact-list purchasing, scraping, enrichment, or list-upload feature;
- accept message requests only from the client's authenticated HighLevel installation;
- require an explicit, documented contact eligibility state before sending when the available HighLevel APIs and agreed workflow permit it;
- enforce STOP and other reasonable revocation requests through a durable suppression record;
- prevent ordinary users from bypassing suppression;
- identify the sender and support HELP/opt-out behavior as required by the approved campaign;
- retain message, delivery, suppression, and administrative audit events;
- include rate limits and an operator-controlled emergency sending stop;
- contain no number rotation, snowshoeing, content mutation, or other carrier-filter-evasion behavior;
- use client-owned HighLevel, messaging-provider, hosting, and billing accounts wherever practical;
- require the client to approve the documented campaign purpose and sending rules before production enablement.

These controls reduce risk and demonstrate intended use, but they are not a guarantee of legal immunity. The written client agreement must separately allocate responsibility and should be reviewed by qualified counsel.

## Required client evidence

Request sanitized examples rather than customer PII:

1. The public opt-in URL or screenshots of every opt-in step.
2. The exact disclosure and checkbox language shown when the number is submitted.
3. The privacy policy and terms linked from the opt-in flow.
4. A redacted export showing the consent evidence retained for a few example contacts.
5. The current A2P brand and campaign registration, including declared use case, sample messages, and call-to-action/opt-in description.
6. A description of every lead source and whether sources are mixed.
7. Existing STOP/DND/suppression data and how it is enforced.
8. Representative first messages, follow-ups, and response workflows.

## Unresolved business requirements

- Final provider: Telnyx or Twilio.
- Number type and number ownership.
- Whether the existing number must be retained.
- Exact campaign creation and scheduling workflow in HighLevel.
- Quiet hours and recipient timezone behavior.
- Follow-up sequence and stopping conditions.
- Human assignment and response workflow.
- MMS requirement.
- Number of simultaneous campaigns.
- Required reporting and monitoring.
- Pilot size and production ramp criteria.
- Migration cutover and rollback requirements.

## Current scope gate

The described postcard → inbound text → affirmative subscription flow makes a consent-based integration plausible, so general architecture and development can proceed. Before A2P submission and production enablement, the client must provide the exact opt-in flow and confirm how completed consent is distinguished from a mere inquiry in the CSV or source system. Any contacts from other sources must have an equally documented eligibility path or remain excluded.
