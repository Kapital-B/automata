-- Project extraction debounce (addendum-project-renovation.md §6.1).
-- Due-ness is derived from assignment timestamps rather than a queue or a
-- counter; this records only when extraction last succeeded, so a project with
-- correspondence newer than this value has work waiting.
ALTER TABLE projects ADD COLUMN IF NOT EXISTS last_extracted_at TIMESTAMPTZ;
