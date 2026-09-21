CREATE TABLE IF NOT EXISTS trusted_keys (
    sender_id  TEXT PRIMARY KEY,
    public_key TEXT NOT NULL,
    added_at   TEXT NOT NULL,
    added_by   TEXT NOT NULL,
    note       TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS bundles (
    bundle_id    TEXT PRIMARY KEY,
    sender_id    TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    received_at  TEXT NOT NULL,
    egress       TEXT NOT NULL,
    body_sha256  TEXT NOT NULL,
    observations INTEGER NOT NULL,
    appended     INTEGER NOT NULL,
    sources      TEXT NOT NULL
);
ALTER TABLE sources ADD COLUMN imported_from TEXT NOT NULL DEFAULT '';
