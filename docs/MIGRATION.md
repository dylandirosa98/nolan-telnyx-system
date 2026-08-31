# Migration and rollback

No live cutover has occurred. The proposed sequence is: inventory HighLevel workflows and suppression data; export and reconcile identifiers; deploy with sending disabled; validate inbound/outbound test traffic against approved accounts; pilot a small cohort; then increment volume with stop/rollback criteria.

Rollback is to pause sending, preserve the database/audit evidence, restore the prior HighLevel provider/workflow, and investigate unsent/failed jobs. Exact HighLevel transfer and API steps remain unverified and are deliberately not invented here.
