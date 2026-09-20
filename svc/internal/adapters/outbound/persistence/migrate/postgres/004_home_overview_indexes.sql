CREATE INDEX IF NOT EXISTS idx_decisions_project_created ON decisions(project_id, created_at);
CREATE INDEX IF NOT EXISTS idx_contradictions_project_created ON contradictions(project_id, created_at);
CREATE INDEX IF NOT EXISTS idx_issues_project_created ON issues(project_id, created_at);
CREATE INDEX IF NOT EXISTS idx_fact_versions_fact_created ON fact_versions(fact_id, created_at);
CREATE INDEX IF NOT EXISTS idx_project_members_user_project ON project_members(user_id, project_id);
