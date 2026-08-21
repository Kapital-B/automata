-- One-off: clear synced email and all derived assistant data.
-- Preserves: accounts (connectors), users/auth, schedule_chains, summary_settings,
--            category_definitions, forward_rules, forward_allowlist.
--
-- Usage (from svc/, server stopped recommended):
--   sqlite3 data.db < scripts/wipe_mailbox_derived_data.sql
--
-- Resets sync cursors so the next job does a full delta replay from Microsoft Graph.

PRAGMA foreign_keys = OFF;

BEGIN IMMEDIATE;

DELETE FROM send_attempts;
DELETE FROM draft_suggestions;
DELETE FROM forward_audit;
DELETE FROM manual_forward_audit;
DELETE FROM fyi_items;
DELETE FROM action_items;
DELETE FROM summary_snapshots;
DELETE FROM message_categories;
DELETE FROM messages;
DELETE FROM job_runs;

UPDATE account_sync_state
SET delta_link = NULL,
    last_synced_at = NULL,
    cursor_json = NULL;

COMMIT;

PRAGMA foreign_keys = ON;
