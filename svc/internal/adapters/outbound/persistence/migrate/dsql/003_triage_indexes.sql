-- Triage read path (addendum-triage-efficiency.md §5.5).
-- The unassigned queue orders mail candidates by received_at within an account,
-- and joins thread assignments on (account_id, conversation_id).
--
-- No DESC: Aurora DSQL's CREATE INDEX grammar has no ASC/DESC on index keys
-- (SQLSTATE 0A000, "specifying sort order not supported for index keys"). The
-- ascending index still serves `ORDER BY received_at DESC`.
CREATE INDEX ASYNC IF NOT EXISTS idx_messages_account_received ON messages(account_id, received_at);
CREATE INDEX ASYNC IF NOT EXISTS idx_thread_assignments_account_conv ON thread_assignments(account_id, conversation_id);
