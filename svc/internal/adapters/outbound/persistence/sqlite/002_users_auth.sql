-- Users, linked identities, mailbox accounts scoped to users, oauth state v2

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY NOT NULL,
    email TEXT NOT NULL COLLATE NOCASE,
    password_hash TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (email)
);

CREATE TABLE IF NOT EXISTS user_identities (
    id TEXT PRIMARY KEY NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (provider IN ('password', 'microsoft', 'google')),
    provider_subject TEXT NOT NULL,
    email_at_link TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (provider, provider_subject)
);

CREATE INDEX IF NOT EXISTS idx_user_identities_user ON user_identities(user_id);
CREATE INDEX IF NOT EXISTS idx_user_identities_email ON user_identities(email_at_link);

INSERT OR IGNORE INTO users (id, email, password_hash, created_at, updated_at) VALUES (
    'a0000001-0000-4000-8000-000000000001',
    'dev@localhost',
    NULL,
    '1970-01-01T00:00:00Z',
    '1970-01-01T00:00:00Z'
);

ALTER TABLE accounts ADD COLUMN user_id TEXT REFERENCES users(id);
UPDATE accounts SET user_id = 'a0000001-0000-4000-8000-000000000001' WHERE user_id IS NULL;

DROP TABLE IF EXISTS oauth_states;

CREATE TABLE oauth_states (
    state TEXT PRIMARY KEY NOT NULL,
    flow TEXT NOT NULL CHECK (flow IN ('m365_mail', 'auth_microsoft', 'auth_google')),
    user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
    payload_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);
