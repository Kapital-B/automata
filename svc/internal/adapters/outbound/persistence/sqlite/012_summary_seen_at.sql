-- Track which messages have been considered by summarize (including filtered-out).

ALTER TABLE messages ADD COLUMN summary_seen_at TEXT;

CREATE INDEX IF NOT EXISTS idx_messages_account_summary_unseen
ON messages(account_id)
WHERE summary_seen_at IS NULL;
