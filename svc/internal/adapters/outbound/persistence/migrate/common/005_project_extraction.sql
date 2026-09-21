-- Project extraction debounce (addendum-project-renovation.md §6.1).
-- Due-ness is derived from assignment timestamps rather than a queue or a
-- counter; this records only when extraction last succeeded, so a project with
-- correspondence newer than this value has work waiting.
ALTER TABLE projects ADD COLUMN IF NOT EXISTS last_extracted_at TIMESTAMPTZ;

-- Issue discard (addendum-project-renovation.md §7.3).
-- Distinct from resolved: resolving means the work is done, discarding means
-- the issue should never have been raised. Conflating them would corrupt the
-- signal for whether extraction is useful. Additive rather than widening the
-- status CHECK, which is unnamed and cannot be safely dropped on DSQL.
ALTER TABLE issues ADD COLUMN IF NOT EXISTS discarded_at TIMESTAMPTZ;

-- Issue provenance (addendum-project-renovation.md §8.3).
-- Extraction now creates issues without an operator, so "where did this come
-- from?" is the first question the project page has to answer. Left free of a
-- CHECK for the same reason as discarded_at. NULL means a row written before
-- extraction existed, all of which were created by hand, so readers treat it
-- as 'human'.
ALTER TABLE issues ADD COLUMN IF NOT EXISTS source TEXT;
