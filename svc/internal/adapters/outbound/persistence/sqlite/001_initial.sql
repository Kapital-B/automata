-- Phase 0–1: accounts, sync state, messages (SQLite)

CREATE TABLE IF NOT EXISTS accounts (
    id TEXT PRIMARY KEY NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT 'm365',
    ms_account_kind TEXT NOT NULL CHECK (ms_account_kind IN ('work', 'personal')),
    graph_tenant_id TEXT,
    primary_email TEXT NOT NULL DEFAULT '',
    msal_home_account_id TEXT,
    connection_status TEXT NOT NULL DEFAULT 'connected' CHECK (connection_status IN ('connected', 'error', 'expired')),
    last_error TEXT,
    token_ciphertext BLOB,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS account_sync_state (
    account_id TEXT PRIMARY KEY NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    delta_link TEXT,
    last_synced_at TEXT,
    cursor_json TEXT
);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY NOT NULL,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    provider_message_id TEXT NOT NULL,
    conversation_id TEXT,
    received_at TEXT NOT NULL,
    subject TEXT NOT NULL DEFAULT '',
    from_json TEXT NOT NULL DEFAULT '{}',
    to_cc_preview TEXT,
    body_text TEXT,
    body_fetched_at TEXT,
    has_attachments INTEGER NOT NULL DEFAULT 0,
    raw_etag TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (account_id, provider_message_id)
);

CREATE INDEX IF NOT EXISTS idx_messages_account ON messages(account_id);

CREATE TABLE IF NOT EXISTS oauth_states (
    state TEXT PRIMARY KEY NOT NULL,
    ms_account_kind TEXT NOT NULL,
    label_hint TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS job_runs (
    id TEXT PRIMARY KEY NOT NULL,
    account_id TEXT REFERENCES accounts(id) ON DELETE SET NULL,
    job_type TEXT NOT NULL CHECK (job_type IN ('sync', 'summarize', 'categorize', 'forward_rules', 'draft_suggest')),
    trigger_kind TEXT NOT NULL CHECK (trigger_kind IN ('schedule', 'api')),
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'success', 'failed', 'cancelled')),
    time_window_start TEXT,
    time_window_end TEXT,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    error_message TEXT,
    meta_json TEXT NOT NULL DEFAULT '{}'
);
