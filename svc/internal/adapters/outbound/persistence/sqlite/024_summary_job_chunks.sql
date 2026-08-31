CREATE TABLE IF NOT EXISTS summary_job_chunks (
    id TEXT PRIMARY KEY NOT NULL,
    run_id TEXT NOT NULL REFERENCES job_runs(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    phase TEXT NOT NULL,
    message_ids_json TEXT NOT NULL DEFAULT '[]',
    payload_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_summary_job_chunks_run_phase_chunk
    ON summary_job_chunks(run_id, phase, chunk_index);
CREATE INDEX IF NOT EXISTS idx_summary_job_chunks_run
    ON summary_job_chunks(run_id, chunk_index);
