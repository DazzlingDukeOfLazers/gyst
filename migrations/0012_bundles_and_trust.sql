-- Signed transfer bundles: who is trusted to send them, which have been
-- received, and which sources arrived that way.
--
-- A bundle is observations as a message. The receiver verifies the
-- sender's signature against a key it has chosen to trust, records the
-- receipt, and appends the observations to its own log under sources
-- namespaced by sender. Nothing in a bundle is executed and nothing in it
-- is trusted for what it says about visibility; labels are rewritten to
-- name the sender.

BEGIN;

CREATE TABLE IF NOT EXISTS trusted_keys (
    sender_id  TEXT PRIMARY KEY,
    public_key TEXT NOT NULL,       -- base64 Ed25519 public key
    added_at   TIMESTAMPTZ NOT NULL,
    added_by   TEXT NOT NULL,
    note       TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS bundles (
    bundle_id    TEXT PRIMARY KEY,
    sender_id    TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    received_at  TIMESTAMPTZ NOT NULL,
    egress       TEXT NOT NULL,
    body_sha256  TEXT NOT NULL,
    observations INT NOT NULL,
    appended     INT NOT NULL,
    sources      JSONB NOT NULL
);

ALTER TABLE sources ADD COLUMN IF NOT EXISTS imported_from TEXT NOT NULL DEFAULT '';

COMMIT;
