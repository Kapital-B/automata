ALTER TABLE draft_suggestions ADD COLUMN status TEXT NOT NULL DEFAULT 'ready';
ALTER TABLE draft_suggestions ADD COLUMN sent_at TEXT;
ALTER TABLE draft_suggestions ADD COLUMN discarded_at TEXT;
ALTER TABLE draft_suggestions ADD COLUMN updated_at TEXT;

CREATE TABLE IF NOT EXISTS send_attempts (
    id TEXT PRIMARY KEY NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    draft_id TEXT NOT NULL REFERENCES draft_suggestions(id) ON DELETE CASCADE,
    message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('success', 'failed')),
    provider_message_id TEXT,
    error_message TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_send_attempts_user_created ON send_attempts(user_id, created_at);
