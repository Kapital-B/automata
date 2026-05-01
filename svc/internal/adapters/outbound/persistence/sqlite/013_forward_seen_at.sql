-- Track which messages have been considered by forward rules.

ALTER TABLE messages ADD COLUMN forward_seen_at TEXT;

CREATE INDEX IF NOT EXISTS idx_messages_account_forward_unseen
ON messages(account_id)
WHERE forward_seen_at IS NULL;
