CREATE TABLE IF NOT EXISTS manual_forward_audit (
    id TEXT PRIMARY KEY NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    to_email TEXT NOT NULL,
    comment TEXT,
    status TEXT NOT NULL CHECK (status IN ('forwarded', 'failed')),
    reason TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_manual_forward_audit_user_created ON manual_forward_audit(user_id, created_at);
