CREATE TABLE IF NOT EXISTS forward_allowlist (
    id TEXT PRIMARY KEY NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(user_id, email)
);

CREATE TABLE IF NOT EXISTS forward_rules (
    id TEXT PRIMARY KEY NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    mode TEXT NOT NULL CHECK (mode IN ('logic', 'llm')),
    condition_json TEXT NOT NULL,
    forward_to TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_forward_rules_user_account ON forward_rules(user_id, account_id, enabled);

CREATE TABLE IF NOT EXISTS forward_audit (
    id TEXT PRIMARY KEY NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    rule_id TEXT NOT NULL REFERENCES forward_rules(id) ON DELETE CASCADE,
    run_id TEXT NOT NULL REFERENCES job_runs(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('forwarded', 'skipped', 'failed')),
    reason TEXT,
    created_at TEXT NOT NULL,
    UNIQUE(message_id, rule_id)
);

CREATE INDEX IF NOT EXISTS idx_forward_audit_user_created ON forward_audit(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_forward_audit_run ON forward_audit(run_id);
