CREATE TABLE IF NOT EXISTS organisations (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL DEFAULT 'Personal',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

INSERT INTO organisations (id, name, created_at, updated_at)
VALUES (
    '20000000-0000-4000-8000-000000000001',
    'Personal',
    '1970-01-01T00:00:00Z',
    '1970-01-01T00:00:00Z'
)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT,
    home_organisation_id UUID REFERENCES organisations(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

INSERT INTO users (id, email, password_hash, home_organisation_id, created_at, updated_at)
VALUES (
    'a0000001-0000-4000-8000-000000000001',
    'dev@localhost',
    NULL,
    '20000000-0000-4000-8000-000000000001',
    '1970-01-01T00:00:00Z',
    '1970-01-01T00:00:00Z'
)
ON CONFLICT (id) DO NOTHING;

UPDATE users
SET home_organisation_id = COALESCE(home_organisation_id, '20000000-0000-4000-8000-000000000001')
WHERE id = 'a0000001-0000-4000-8000-000000000001';

CREATE TABLE IF NOT EXISTS organisation_members (
    id UUID PRIMARY KEY,
    organisation_id UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_role TEXT NOT NULL CHECK (org_role IN ('owner', 'member')),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organisation_id, user_id)
);

INSERT INTO organisation_members (id, organisation_id, user_id, org_role, created_at)
VALUES (
    '20000000-0000-4000-8000-000000000002',
    '20000000-0000-4000-8000-000000000001',
    'a0000001-0000-4000-8000-000000000001',
    'owner',
    '1970-01-01T00:00:00Z'
)
ON CONFLICT (organisation_id, user_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS user_identities (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (provider IN ('password', 'microsoft', 'google')),
    provider_subject TEXT NOT NULL,
    email_at_link TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (provider, provider_subject)
);

CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT 'm365',
    ms_account_kind TEXT NOT NULL CHECK (ms_account_kind IN ('work', 'personal')),
    graph_tenant_id TEXT,
    primary_email TEXT NOT NULL DEFAULT '',
    msal_home_account_id TEXT,
    connection_status TEXT NOT NULL DEFAULT 'connected' CHECK (connection_status IN ('connected', 'error', 'expired')),
    last_error TEXT,
    token_ciphertext BYTEA,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS account_sync_state (
    account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    delta_link TEXT,
    last_synced_at TIMESTAMPTZ,
    cursor_json TEXT
);

CREATE TABLE IF NOT EXISTS messages (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    provider_message_id TEXT NOT NULL,
    conversation_id TEXT,
    received_at TIMESTAMPTZ NOT NULL,
    subject TEXT NOT NULL DEFAULT '',
    from_json TEXT NOT NULL DEFAULT '{}',
    to_json TEXT NOT NULL DEFAULT '[]',
    cc_json TEXT NOT NULL DEFAULT '[]',
    to_cc_preview TEXT,
    body_text TEXT,
    body_fetched_at TIMESTAMPTZ,
    has_attachments BOOLEAN NOT NULL DEFAULT FALSE,
    raw_etag TEXT,
    summary_seen_at TIMESTAMPTZ,
    forward_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (account_id, provider_message_id)
);

CREATE TABLE IF NOT EXISTS oauth_states (
    state TEXT PRIMARY KEY,
    flow TEXT NOT NULL CHECK (flow IN ('m365_mail', 'auth_microsoft', 'auth_google', 'slack_connector')),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    payload_json TEXT NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS auth_sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS category_definitions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    slug TEXT NOT NULL,
    display_name TEXT NOT NULL,
    definition TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (user_id, slug)
);

INSERT INTO category_definitions (id, user_id, slug, display_name, definition, sort_order, created_at, updated_at)
VALUES
    ('10000000-0000-4000-8000-000000000001', 'a0000001-0000-4000-8000-000000000001', 'important', 'Important', 'Emails requiring timely attention, decisions, or follow-up.', 10, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
    ('10000000-0000-4000-8000-000000000002', 'a0000001-0000-4000-8000-000000000001', 'finance', 'Finance', 'Invoices, receipts, statements, payments, taxes, banking, or accounting.', 20, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
    ('10000000-0000-4000-8000-000000000003', 'a0000001-0000-4000-8000-000000000001', 'personal', 'Personal', 'Personal correspondence, family, friends, appointments, or non-work life admin.', 30, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
    ('10000000-0000-4000-8000-000000000004', 'a0000001-0000-4000-8000-000000000001', 'newsletter', 'Newsletter', 'Subscriptions, digests, marketing updates, announcements, or recurring publications.', 40, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
    ('10000000-0000-4000-8000-000000000005', 'a0000001-0000-4000-8000-000000000001', 'spam', 'Spam', 'Unwanted, suspicious, deceptive, or low-value promotional email.', 50, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
    ('10000000-0000-4000-8000-000000000006', 'a0000001-0000-4000-8000-000000000001', 'other', 'Other', 'Anything that does not clearly fit another category.', 60, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS message_categories (
    id UUID PRIMARY KEY,
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES category_definitions(id) ON DELETE CASCADE,
    source TEXT NOT NULL CHECK (source IN ('llm', 'rule', 'user')),
    confidence DOUBLE PRECISION,
    run_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (message_id, source)
);

CREATE TABLE IF NOT EXISTS summary_settings (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    include_category_slugs TEXT NOT NULL DEFAULT '[]',
    exclude_category_slugs TEXT NOT NULL DEFAULT '[]',
    chunk_size INTEGER NOT NULL DEFAULT 12,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS summary_snapshots (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id UUID REFERENCES accounts(id) ON DELETE CASCADE,
    run_id UUID NOT NULL,
    account_scope_id UUID GENERATED ALWAYS AS (COALESCE(account_id, '00000000-0000-0000-0000-000000000000'::uuid)) STORED,
    window_start TIMESTAMPTZ NOT NULL,
    window_end TIMESTAMPTZ NOT NULL,
    general_summary TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (run_id, account_scope_id, window_start, window_end)
);

CREATE TABLE IF NOT EXISTS summary_job_chunks (
    id UUID PRIMARY KEY,
    run_id UUID NOT NULL,
    account_id UUID REFERENCES accounts(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    window_end TIMESTAMPTZ NOT NULL,
    message_count INTEGER NOT NULL DEFAULT 0,
    summary_text TEXT NOT NULL DEFAULT '',
    payload_json TEXT NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (run_id, chunk_index)
);

CREATE TABLE IF NOT EXISTS action_items (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    run_id UUID NOT NULL,
    text TEXT NOT NULL,
    due_at TIMESTAMPTZ,
    status TEXT NOT NULL CHECK (status IN ('open', 'done')),
    actioned_at TIMESTAMPTZ,
    auto_draft_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS fyi_items (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    run_id UUID NOT NULL,
    text TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS forward_allowlist (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (user_id, email)
);

CREATE TABLE IF NOT EXISTS forward_rules (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    mode TEXT NOT NULL CHECK (mode IN ('logic', 'llm')),
    condition_json TEXT NOT NULL,
    forward_to TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS forward_audit (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    rule_id UUID NOT NULL REFERENCES forward_rules(id) ON DELETE CASCADE,
    run_id UUID NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('forwarded', 'skipped', 'failed')),
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (message_id, rule_id)
);

CREATE TABLE IF NOT EXISTS manual_forward_audit (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    to_email TEXT NOT NULL,
    comment TEXT,
    status TEXT NOT NULL CHECK (status IN ('forwarded', 'failed')),
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS contacts (
    id UUID PRIMARY KEY,
    organisation_id UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL DEFAULT '',
    company TEXT,
    merged_into_contact_id UUID REFERENCES contacts(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS contact_identities (
    id UUID PRIMARY KEY,
    organisation_id UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    contact_id UUID NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('email', 'phone', 'display_name_hint')),
    value_normalized TEXT NOT NULL,
    value_raw TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organisation_id, kind, value_normalized)
);

CREATE TABLE IF NOT EXISTS contact_profile_links (
    id UUID PRIMARY KEY,
    organisation_id UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    contact_id UUID NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (contact_id),
    UNIQUE (organisation_id, user_id)
);

CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY,
    organisation_id UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    code TEXT NOT NULL,
    description TEXT,
    client TEXT,
    keywords_json TEXT NOT NULL DEFAULT '[]',
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organisation_id, code)
);

CREATE TABLE IF NOT EXISTS project_members (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT '',
    discipline TEXT,
    responsibilities TEXT,
    current_scope TEXT,
    approval_authority TEXT,
    out_of_scope TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (project_id, user_id)
);

CREATE TABLE IF NOT EXISTS project_participants (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    contact_id UUID NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    first_seen_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (project_id, contact_id)
);

CREATE TABLE IF NOT EXISTS manual_items (
    id UUID PRIMARY KEY,
    organisation_id UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    channel TEXT NOT NULL CHECK (channel IN ('whatsapp', 'teams', 'sms', 'call', 'meeting', 'note')),
    occurred_at TIMESTAMPTZ NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    body_text TEXT NOT NULL,
    project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
    assignment_status TEXT NOT NULL CHECK (assignment_status IN ('committed', 'provisional', 'unassigned')),
    assignment_reason TEXT,
    assignment_source TEXT CHECK (assignment_source IS NULL OR assignment_source IN ('user', 'rule', 'llm')),
    created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS correspondence_participants (
    id UUID PRIMARY KEY,
    organisation_id UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    contact_id UUID NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('from', 'to', 'cc', 'participant')),
    message_id UUID REFERENCES messages(id) ON DELETE CASCADE,
    manual_item_id UUID REFERENCES manual_items(id) ON DELETE CASCADE,
    CHECK (
        (message_id IS NOT NULL AND manual_item_id IS NULL) OR
        (message_id IS NULL AND manual_item_id IS NOT NULL)
    ),
    UNIQUE (contact_id, role, message_id),
    UNIQUE (contact_id, role, manual_item_id)
);

CREATE TABLE IF NOT EXISTS thread_assignments (
    id UUID PRIMARY KEY,
    organisation_id UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    conversation_id TEXT NOT NULL,
    project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
    status TEXT NOT NULL CHECK (status IN ('committed', 'provisional')),
    confidence DOUBLE PRECISION,
    reason TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL CHECK (source IN ('user', 'rule', 'llm')),
    run_id UUID,
    assigned_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (account_id, conversation_id)
);

CREATE TABLE IF NOT EXISTS message_assignment_overrides (
    message_id UUID PRIMARY KEY REFERENCES messages(id) ON DELETE CASCADE,
    organisation_id UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
    status TEXT NOT NULL CHECK (status IN ('committed', 'provisional')),
    confidence DOUBLE PRECISION,
    reason TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL CHECK (source IN ('user', 'rule', 'llm')),
    run_id UUID,
    assigned_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS schedule_chains (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    account_id UUID REFERENCES accounts(id) ON DELETE CASCADE,
    jobs_json TEXT NOT NULL,
    interval_minutes INTEGER NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS issues (
    id UUID PRIMARY KEY,
    organisation_id UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    current_position_note TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('open', 'awaiting_input', 'resolved')),
    assignee_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    assignee_contact_id UUID REFERENCES contacts(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CHECK (assignee_user_id IS NULL OR assignee_contact_id IS NULL)
);

CREATE TABLE IF NOT EXISTS issue_items (
    id UUID PRIMARY KEY,
    issue_id UUID NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    message_id UUID REFERENCES messages(id) ON DELETE CASCADE,
    manual_item_id UUID REFERENCES manual_items(id) ON DELETE CASCADE,
    added_at TIMESTAMPTZ NOT NULL,
    CHECK (
        (message_id IS NOT NULL AND manual_item_id IS NULL) OR
        (message_id IS NULL AND manual_item_id IS NOT NULL)
    ),
    UNIQUE (message_id),
    UNIQUE (manual_item_id)
);

CREATE TABLE IF NOT EXISTS facts (
    id UUID PRIMARY KEY,
    organisation_id UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    issue_id UUID REFERENCES issues(id) ON DELETE SET NULL,
    subject_key TEXT NOT NULL,
    label TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (project_id, subject_key)
);

CREATE TABLE IF NOT EXISTS interpretations (
    id UUID PRIMARY KEY,
    organisation_id UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    run_id UUID,
    status TEXT NOT NULL CHECK (status IN ('pending', 'accepted', 'dismissed', 'expired')),
    payload_json TEXT NOT NULL,
    confidence DOUBLE PRECISION,
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS fact_versions (
    id UUID PRIMARY KEY,
    fact_id UUID NOT NULL REFERENCES facts(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('proposed', 'active', 'superseded', 'rejected')),
    value_json TEXT NOT NULL,
    value_text TEXT NOT NULL,
    unit TEXT,
    source TEXT NOT NULL CHECK (source IN ('user', 'rule', 'llm')),
    confidence DOUBLE PRECISION,
    interpretation_id UUID,
    supersedes_version_id UUID REFERENCES fact_versions(id) ON DELETE SET NULL,
    superseded_by_version_id UUID REFERENCES fact_versions(id) ON DELETE SET NULL,
    superseded_at TIMESTAMPTZ,
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    active_fact_id UUID GENERATED ALWAYS AS (CASE WHEN status = 'active' THEN fact_id ELSE NULL END) STORED,
    UNIQUE (active_fact_id)
);

CREATE TABLE IF NOT EXISTS fact_evidence (
    id UUID PRIMARY KEY,
    fact_version_id UUID NOT NULL REFERENCES fact_versions(id) ON DELETE CASCADE,
    message_id UUID REFERENCES messages(id) ON DELETE CASCADE,
    manual_item_id UUID REFERENCES manual_items(id) ON DELETE CASCADE,
    added_at TIMESTAMPTZ NOT NULL,
    CHECK (
        (message_id IS NOT NULL AND manual_item_id IS NULL) OR
        (message_id IS NULL AND manual_item_id IS NOT NULL)
    )
);

CREATE TABLE IF NOT EXISTS connector_accounts (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (provider IN ('slack', 'teams', 'whatsapp', 'sms')),
    label TEXT NOT NULL,
    external_tenant_id TEXT,
    connection_status TEXT NOT NULL CHECK (connection_status IN ('connected', 'error', 'disconnected')),
    last_error TEXT,
    scopes_json TEXT NOT NULL DEFAULT '[]',
    token_ciphertext BYTEA NOT NULL,
    last_synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS connector_bindings (
    id UUID PRIMARY KEY,
    connector_account_id UUID NOT NULL REFERENCES connector_accounts(id) ON DELETE CASCADE,
    organisation_id UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    external_channel_id TEXT NOT NULL,
    project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
    label TEXT NOT NULL DEFAULT '',
    sync_cursor TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (connector_account_id, external_channel_id)
);

CREATE TABLE IF NOT EXISTS connector_messages (
    id UUID PRIMARY KEY,
    connector_account_id UUID NOT NULL REFERENCES connector_accounts(id) ON DELETE CASCADE,
    organisation_id UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
    provider_event_id TEXT NOT NULL,
    external_channel_id TEXT NOT NULL,
    title TEXT NOT NULL,
    body_text TEXT NOT NULL,
    author_label TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL,
    meta_json TEXT NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (connector_account_id, provider_event_id)
);

CREATE TABLE IF NOT EXISTS interpretation_sources (
    id UUID PRIMARY KEY,
    interpretation_id UUID NOT NULL REFERENCES interpretations(id) ON DELETE CASCADE,
    message_id UUID REFERENCES messages(id) ON DELETE CASCADE,
    manual_item_id UUID REFERENCES manual_items(id) ON DELETE CASCADE,
    connector_message_id UUID REFERENCES connector_messages(id) ON DELETE CASCADE,
    CHECK (
        (message_id IS NOT NULL AND manual_item_id IS NULL AND connector_message_id IS NULL) OR
        (message_id IS NULL AND manual_item_id IS NOT NULL AND connector_message_id IS NULL) OR
        (message_id IS NULL AND manual_item_id IS NULL AND connector_message_id IS NOT NULL)
    )
);

CREATE TABLE IF NOT EXISTS decisions (
    id UUID PRIMARY KEY,
    organisation_id UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    issue_id UUID REFERENCES issues(id) ON DELETE SET NULL,
    statement TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('proposed', 'accepted', 'superseded', 'withdrawn')),
    decided_at TIMESTAMPTZ,
    assignee_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    assignee_contact_id UUID REFERENCES contacts(id) ON DELETE SET NULL,
    source TEXT NOT NULL CHECK (source IN ('user', 'rule', 'llm')),
    confidence DOUBLE PRECISION,
    supersedes_decision_id UUID REFERENCES decisions(id) ON DELETE SET NULL,
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CHECK (
        (assignee_user_id IS NULL AND assignee_contact_id IS NULL) OR
        (assignee_user_id IS NOT NULL AND assignee_contact_id IS NULL) OR
        (assignee_user_id IS NULL AND assignee_contact_id IS NOT NULL)
    )
);

CREATE TABLE IF NOT EXISTS decision_evidence (
    id UUID PRIMARY KEY,
    decision_id UUID NOT NULL REFERENCES decisions(id) ON DELETE CASCADE,
    message_id UUID REFERENCES messages(id) ON DELETE CASCADE,
    manual_item_id UUID REFERENCES manual_items(id) ON DELETE CASCADE,
    added_at TIMESTAMPTZ NOT NULL,
    CHECK (
        (message_id IS NOT NULL AND manual_item_id IS NULL) OR
        (message_id IS NULL AND manual_item_id IS NOT NULL)
    )
);

CREATE TABLE IF NOT EXISTS contradictions (
    id UUID PRIMARY KEY,
    organisation_id UUID NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('open', 'resolved')),
    summary TEXT NOT NULL,
    resolution_note TEXT,
    resolved_at TIMESTAMPTZ,
    resolved_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS contradiction_sides (
    id UUID PRIMARY KEY,
    contradiction_id UUID NOT NULL REFERENCES contradictions(id) ON DELETE CASCADE,
    fact_version_id UUID REFERENCES fact_versions(id) ON DELETE SET NULL,
    decision_id UUID REFERENCES decisions(id) ON DELETE SET NULL,
    CHECK (fact_version_id IS NOT NULL OR decision_id IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS draft_suggestions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    action_item_id UUID NOT NULL REFERENCES action_items(id) ON DELETE CASCADE,
    run_id UUID NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    model TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'ready',
    sent_at TIMESTAMPTZ,
    discarded_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (run_id, message_id)
);

CREATE TABLE IF NOT EXISTS send_attempts (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    draft_id UUID NOT NULL REFERENCES draft_suggestions(id) ON DELETE CASCADE,
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('success', 'failed')),
    provider_message_id TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL
);
