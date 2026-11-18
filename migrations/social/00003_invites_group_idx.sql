-- +goose Up
-- ListGroupInvites filters invites by group.
CREATE INDEX idx_invites_group ON invites (group_id) WHERE group_id IS NOT NULL;

-- +goose Down
DROP INDEX idx_invites_group;
