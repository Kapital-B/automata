-- Not-project-related correspondence (addendum-triage-not-relevant.md §5.2).
-- project_id = NULL already means "Unassigned", which is the state that keeps
-- an item in the triage queue, so dismissing something needs its own column.
-- Additive rather than a new status value: every CHECK in the baseline is
-- inline and unnamed, and widening one on DSQL would mean dropping a
-- constraint by its generated name.
ALTER TABLE thread_assignments ADD COLUMN IF NOT EXISTS not_relevant_at TIMESTAMPTZ;
ALTER TABLE message_assignment_overrides ADD COLUMN IF NOT EXISTS not_relevant_at TIMESTAMPTZ;
ALTER TABLE manual_items ADD COLUMN IF NOT EXISTS not_relevant_at TIMESTAMPTZ;
