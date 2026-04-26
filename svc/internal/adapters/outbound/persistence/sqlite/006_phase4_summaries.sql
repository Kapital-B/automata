CREATE TABLE IF NOT EXISTS summary_settings (
    user_id TEXT PRIMARY KEY NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    include_category_slugs TEXT NOT NULL DEFAULT '[]',
    exclude_category_slugs TEXT NOT NULL DEFAULT '[]',
    chunk_size INTEGER NOT NULL DEFAULT 12,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS summary_snapshots (
    id TEXT PRIMARY KEY NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id TEXT REFERENCES accounts(id) ON DELETE CASCADE,
    run_id TEXT NOT NULL REFERENCES job_runs(id) ON DELETE CASCADE,
    window_start TEXT NOT NULL,
    window_end TEXT NOT NULL,
    general_summary TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_summary_snapshots_user_account ON summary_snapshots(user_id, account_id, created_at);
CREATE INDEX IF NOT EXISTS idx_summary_snapshots_run ON summary_snapshots(run_id);

CREATE TABLE IF NOT EXISTS action_items (
    id TEXT PRIMARY KEY NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    run_id TEXT NOT NULL REFERENCES job_runs(id) ON DELETE CASCADE,
    text TEXT NOT NULL,
    due_at TEXT,
    status TEXT NOT NULL CHECK (status IN ('open', 'done')),
    actioned_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_action_items_user_status ON action_items(user_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_action_items_run ON action_items(run_id);

CREATE TABLE IF NOT EXISTS fyi_items (
    id TEXT PRIMARY KEY NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    run_id TEXT NOT NULL REFERENCES job_runs(id) ON DELETE CASCADE,
    text TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_fyi_items_run ON fyi_items(run_id);
CREATE INDEX IF NOT EXISTS idx_fyi_items_user_created ON fyi_items(user_id, created_at);
