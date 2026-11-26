CREATE TABLE IF NOT EXISTS users (
    id            text PRIMARY KEY,
    name          text NOT NULL,
    role          text NOT NULL CHECK (role IN ('worker', 'supervisor')),
    password_hash bytea NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    token      text PRIMARY KEY,
    user_id    text NOT NULL REFERENCES users(id),
    expires_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS equipment (
    id      text PRIMARY KEY,
    name    text NOT NULL,
    retired boolean NOT NULL DEFAULT false
);

CREATE TABLE IF NOT EXISTS inspections (
    id           uuid PRIMARY KEY,
    root_id      uuid NOT NULL REFERENCES inspections(id),
    revision     integer NOT NULL,
    equipment_id text NOT NULL REFERENCES equipment(id),
    result       text NOT NULL CHECK (result IN ('normal', 'abnormal')),
    memo         text NOT NULL DEFAULT '',
    inspected_at timestamptz NOT NULL,
    author_id    text NOT NULL REFERENCES users(id),
    content_hash bytea NOT NULL,
    received_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (revision >= 0 AND ((revision = 0) = (root_id = id))),
    UNIQUE (root_id, revision)
);
