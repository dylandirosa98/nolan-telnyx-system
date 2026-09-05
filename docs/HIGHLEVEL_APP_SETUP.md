# HighLevel private SMS provider setup

This app lets staff continue using HighLevel and the LeadConnector mobile app normally while Telnyx transports SMS. It is a private backend connector, not a replacement user interface.

## App profile

- Name: `Telnyx SMS Connector`
- App type: `Private`
- Target user: `Sub-account`
- Who can install: `Both Agency and Sub-account`
- Listing type: `White-Label`
- Category: `Communication`
- Tagline: `Connects HighLevel conversations to Telnyx for SMS messaging.`
- Company: the developer's own business or personal name unless the client has authorized use of its identity

## OAuth auth settings

Add exactly this redirect URL:

`https://api-production-55968.up.railway.app/oauth/highlevel/callback`

Select these scopes:

- `conversations/message.write`
- `conversations/message.readonly`
- `conversations.readonly`
- `conversations.write`
- `contacts.readonly`
- `contacts.write`

Under Client Keys, create a key named `Production OAuth`. Store its Client ID and one-time Client Secret outside Git. Do not use the Shared Secret Key as the OAuth Client Secret.

## SMS Conversation Provider

Create a Conversation Provider with:

- Name: `Telnyx SMS`
- Type: `SMS`
- Delivery URL: `https://api-production-55968.up.railway.app/webhooks/highlevel/outbound`
- `Is this a Custom Conversation Provider`: **unchecked**

Leaving the custom-provider option unchecked makes this a replacement for LC Phone/Twilio and preserves support for the normal SMS composer, workflows, bulk actions, and mobile app. Record the generated `conversationProviderId`; it becomes the Railway variable `HIGHLEVEL_CONVERSATION_PROVIDER_ID`.

## Installation sequence

Do not open the installation URL until the OAuth Client ID and Client Secret have been installed on both Railway application services and the API has been redeployed successfully.

1. Generate the state-bound HighLevel authorization URL through the service's authenticated `/oauth/highlevel/start` route. Do not use a code-only callback.
2. The client agency owner/admin opens that URL and selects the intended sub-account.
3. HighLevel redirects to the configured callback. A successful response says `connected` and includes the selected location ID.
4. Save that location ID as Railway variable `HIGHLEVEL_LOCATION_ID` on both API and worker services.
5. In the selected sub-account, navigate to **Settings → Phone Numbers → Advanced Settings → SMS Provider**, choose `Telnyx SMS`, and save.
6. Keep `ENABLE_SENDING=false` and the database sending switch paused until Telnyx registration, number assignment, signed-webhook verification, and a controlled pilot are complete.

## CSV import then text

This connector does not accept CSV files. Import opted-in contacts in HighLevel, then send from Conversations, bulk actions, or workflows. HighLevel calls the Conversation Provider delivery URL for each SMS; this service queues, suppresses STOP numbers, and sends through Telnyx after sending is enabled.

## Acceptance test after Telnyx is ready

1. Verify API health/readiness and that unsigned provider webhooks are rejected.
2. Send one approved SMS from the HighLevel web conversation screen.
3. Confirm it is queued once, sent once by Telnyx, and receives a final delivery status in HighLevel.
4. Reply from the handset and confirm the reply appears in the same HighLevel/LeadConnector conversation.
5. Repeat from the LeadConnector mobile app.
6. Send `STOP`; verify HighLevel DND, local suppression, and cancellation of pending sends.
7. Keep the pilot at low volume until retries, duplicate callbacks, quiet hours, logs, and rollback have been observed.
