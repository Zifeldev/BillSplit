-- +goose Up
-- user ids come from the auth service: no cross-service foreign keys.

-- One row per pair, user_low < user_high.
CREATE TABLE friendships (
    user_low     UUID        NOT NULL,
    user_high    UUID        NOT NULL,
    requested_by UUID        NOT NULL,
    status       SMALLINT    NOT NULL DEFAULT 1, -- 1 pending, 2 accepted
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    accepted_at  TIMESTAMPTZ,
    PRIMARY KEY (user_low, user_high),
    CHECK (user_low < user_high)
);
CREATE INDEX idx_friendships_high ON friendships (user_high);

CREATE TABLE groups (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    owner_id   UUID        NOT NULL,
    currency   CHAR(3)     NOT NULL DEFAULT 'USD',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE group_members (
    group_id  UUID        NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    user_id   UUID        NOT NULL,
    role      TEXT        NOT NULL DEFAULT 'member',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, user_id)
);
CREATE INDEX idx_group_members_user ON group_members (user_id);

-- Invite links: kind 1 = friend, 2 = group.
CREATE TABLE invites (
    token      TEXT PRIMARY KEY,
    kind       SMALLINT    NOT NULL,
    inviter_id UUID        NOT NULL,
    group_id   UUID REFERENCES groups (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    max_uses   INT         NOT NULL DEFAULT 0,
    uses       INT         NOT NULL DEFAULT 0,
    revoked    BOOLEAN     NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((kind = 2) = (group_id IS NOT NULL))
);

-- +goose Down
DROP TABLE invites, group_members, groups, friendships;
