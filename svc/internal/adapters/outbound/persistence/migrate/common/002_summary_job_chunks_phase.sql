-- Replace the legacy window-based summary_job_chunks schema with the phase-based
-- chunk rows used by summarize jobs (map/reduce partials).
DROP TABLE IF EXISTS summary_job_chunks;

CREATE TABLE summary_job_chunks (
    id UUID PRIMARY KEY,
    run_id UUID NOT NULL,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    phase TEXT NOT NULL,
    message_ids_json TEXT NOT NULL DEFAULT '[]',
    payload_json TEXT NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (run_id, phase, chunk_index)
);
