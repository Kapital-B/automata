-- To-dos filed to a project are shared with everyone on it, and any member can
-- close one. actioned_at says when; this says who, which is no longer always
-- the owner. No foreign key: DSQL does not enforce them, and a departed user
-- should not erase the record that they closed something.
ALTER TABLE action_items ADD COLUMN IF NOT EXISTS actioned_by_user_id UUID;
