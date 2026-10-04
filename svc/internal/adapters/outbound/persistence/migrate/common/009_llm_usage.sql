-- One row per model call, for metering spend per user and feature. Cost is
-- not stored: it is derived from a price table at read time, so price changes
-- need no backfill. user_id and account_id are nullable because a call made
-- outside any user's scope is still recorded rather than lost. No foreign
-- keys: DSQL does not enforce them, and usage must outlive a deleted account.
CREATE TABLE IF NOT EXISTS llm_usage (
    id UUID PRIMARY KEY,
    user_id UUID,
    account_id UUID,
    feature TEXT NOT NULL,
    model TEXT NOT NULL,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL
);
