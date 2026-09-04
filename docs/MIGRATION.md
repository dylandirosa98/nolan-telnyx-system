# Migration and rollback

## Scope

The legacy HighLevel location remains active for its existing contacts, opportunities, conversations, and appointments. The new client-owned location receives configuration only; no live CRM records or message history are migrated.

## Verified configuration inventory

Read-only API access to the representative legacy source and new destination has been exercised successfully. The current structural counts are:

| Asset | Legacy source | New destination |
| --- | ---: | ---: |
| Pipelines | 4 | 1 |
| Custom fields | 153 | 0 |
| Custom values | 70 | 0 |
| Tags | 210 | 3 |
| Calendars | 10 | 1 |
| Users | 2 | 2 |
| Workflows | 17 | 0 |
| Forms | 10 | 0 |
| Surveys | 0 | 0 |
| Funnels | 4 | 0 |

The inventory deliberately did not query or export contacts, opportunities, conversations, appointments, or message history.

## Migration method

A HighLevel Snapshot shared by the legacy white-label agency is the preferred migration mechanism. The asset volume and the presence of workflows, forms, and funnels make manual API reconstruction slower and less reliable, and not every HighLevel asset has a complete public create/update API.

The migration sequence is:

1. Copy the API-addressable core structure into the destination.
2. Re-run the structural inventory and compare source and destination assets by normalized name and type, not by provider-generated IDs.
3. Reconnect destination-specific dependencies such as users, calendars, domains, integrations, phone configuration, and workflow references.
4. Confirm that no contacts, opportunities, conversations, appointments, or historical messages were imported.
5. Test forms, pipeline transitions, workflow enrollment, and messaging actions using synthetic records only.
6. Keep the legacy locations unchanged and active while their existing leads are completed.

## Executed API migration

The following configuration migration has been executed against the new destination and read back for verification:

- All four legacy pipelines and all 46 stages were created. Pipeline/stage names, ordering, visibility flags, opportunity-probability mode, stage probabilities, and stage colors match the source API representation.
- All 153 custom fields were created with matching model, name, type, placeholder, and option values. The owner accepted importing them without their 14 legacy folder groupings because HighLevel does not expose contact/opportunity field folders through the applicable location API.
- Two multi-file fields had no maximum-file value in the source API response even though the destination API requires one. They were created with a permissive limit of 10 and require UI confirmation.
- The source contained 210 tags but one case-insensitive duplicate. All 209 unique legacy tag names are present in the destination. Three pre-existing destination tags were retained.
- All 70 custom values were reviewed programmatically; every source value was empty. Their names/keys were created in the destination with empty values for later client-specific configuration.
- Source-to-destination ID maps and sanitized review evidence are stored under the ignored local `.local/` directory. No credentials or CRM records are committed.
- The destination's original default pipeline and other defaults were retained rather than deleted automatically.

This execution did not query, export, create, update, or delete contacts, opportunities, conversations, appointments, or message history.

## Workflow and snippet migration constraints

- The source has 17 workflows: 12 published and 5 draft.
- HighLevel's supported workflow API exposes only the workflow list/metadata. It has no endpoint to read workflow triggers/actions or create/clone workflows; direct detail probes returned 404.
- HighLevel's UI can copy a workflow to another sub-account, but only an administrator of the source agency can use that operation.
- The source has 10 SMS snippets. Their names and bodies are readable through `GET /locations/:locationId/templates`.
- The supported template API exposes `locations/templates.readonly` and delete operations, but no create scope. A live POST probe against the destination was rejected with `401` before creating anything.
- Without source-agency administrator help, workflows and snippets must therefore be recreated through authenticated HighLevel UI access. Recreated workflows must remain draft until destination references and Telnyx behavior are tested.

## Cutover and rollback

No live cutover has occurred. After the configuration migration, deploy with sending disabled; validate inbound/outbound test traffic against approved accounts; pilot a small cohort; then increment volume with explicit stop criteria.

Rollback is to disable the new workflows and sending path, preserve the database and audit evidence, leave the legacy locations operating independently, and investigate unsent or failed jobs. The legacy locations are not modified or transferred as part of this migration.
