-- Forward rules: explicit scope, and per-rule verdicts that can be revisited.
--
-- A rule used to see every message nobody had marked forward_seen_at. The
-- first run after creating a rule therefore walked the whole mailbox, and a
-- rule added later never saw existing mail at all, depending on nothing more
-- than whether forwarding had run before.
--
-- apply_from is when a rule starts: set when it is switched on, to that
-- moment for new mail only or to the epoch to include existing mail.
ALTER TABLE forward_rules ADD COLUMN IF NOT EXISTS apply_from TIMESTAMPTZ;

-- forward_audit already holds one verdict per (message, rule). pending marks a
-- verdict that is not final (a send that will be retried, a category rule
-- waiting for categorisation); attempts bounds the retries. New columns
-- rather than new status values: status carries an unnamed CHECK, and
-- widening an unnamed CHECK is what broke the DSQL deploys on PR 9.
ALTER TABLE forward_audit ADD COLUMN IF NOT EXISTS pending BOOLEAN;
ALTER TABLE forward_audit ADD COLUMN IF NOT EXISTS attempts INTEGER;

-- Existing rules carry on from where forwarding left off: just after the
-- newest message a run had checked on their account, or their creation if
-- none had run. Anything older was already decided under the old scheme.
UPDATE forward_rules
SET apply_from = COALESCE(
	(SELECT MAX(m.received_at) + INTERVAL '1 millisecond'
	 FROM messages m
	 WHERE m.account_id = forward_rules.account_id AND m.forward_seen_at IS NOT NULL),
	forward_rules.created_at)
WHERE apply_from IS NULL;
