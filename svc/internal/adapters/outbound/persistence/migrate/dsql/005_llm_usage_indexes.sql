CREATE INDEX ASYNC IF NOT EXISTS idx_llm_usage_user_created ON llm_usage(user_id, created_at);
