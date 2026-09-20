-- Home overview activity feed (addendum-home-overview.md §8.1).
-- fact_versions had no activation time and issues had no resolution time, so
-- those two events could only be dated by approximation. Nullable and not
-- backfilled: readers COALESCE to the old approximation for existing rows,
-- which avoids a large DML backfill inside the migration.
ALTER TABLE fact_versions ADD COLUMN IF NOT EXISTS activated_at TIMESTAMPTZ;
ALTER TABLE issues ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;
