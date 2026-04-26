ALTER TABLE action_items ADD COLUMN auto_draft_seen_at TEXT;

CREATE TABLE IF NOT EXISTS draft_suggestions (
    id TEXT PRIMARY KEY NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    action_item_id TEXT NOT NULL REFERENCES action_items(id) ON DELETE CASCADE,
    run_id TEXT NOT NULL REFERENCES job_runs(id) ON DELETE CASCADE,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    model TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_draft_suggestions_user_created ON draft_suggestions(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_draft_suggestions_action_item ON draft_suggestions(action_item_id);
