CREATE TABLE IF NOT EXISTS category_definitions (
    id TEXT PRIMARY KEY NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS message_categories (
    id TEXT PRIMARY KEY NOT NULL,
    message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    category_id TEXT NOT NULL REFERENCES category_definitions(id) ON DELETE CASCADE,
    source TEXT NOT NULL CHECK (source IN ('llm', 'rule', 'user')),
    confidence REAL,
    run_id TEXT NOT NULL REFERENCES job_runs(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (message_id, source)
);

CREATE INDEX IF NOT EXISTS idx_message_categories_account ON message_categories(account_id);
CREATE INDEX IF NOT EXISTS idx_message_categories_message ON message_categories(message_id);
CREATE INDEX IF NOT EXISTS idx_message_categories_category ON message_categories(category_id);

INSERT OR IGNORE INTO category_definitions (id, slug, display_name, sort_order) VALUES
    ('10000000-0000-4000-8000-000000000001', 'important', 'Important', 10),
    ('10000000-0000-4000-8000-000000000002', 'finance', 'Finance', 20),
    ('10000000-0000-4000-8000-000000000003', 'personal', 'Personal', 30),
    ('10000000-0000-4000-8000-000000000004', 'newsletter', 'Newsletter', 40),
    ('10000000-0000-4000-8000-000000000005', 'spam', 'Spam', 50),
    ('10000000-0000-4000-8000-000000000006', 'other', 'Other', 60);
