-- Explicit assertions by people, and the authority projection over them.
--
-- The top of the membership and identity precedence order is an assertion
-- made by an authorized person. Nothing Gyst infers may override one. An
-- assertion is therefore durable in the way an observation is: it is never
-- edited, and it is never deleted. Changing one's mind is a retraction,
-- recorded on the row with who and why, so the history of what was
-- asserted remains readable.
--
-- The first assertion kinds are about authority: "this file is the
-- authority" and "this copy is not". A manifest declares membership and a
-- profile groups versions; neither says which copy to build from. Only a
-- person does, or nothing does.

BEGIN;

CREATE TABLE IF NOT EXISTS assertions (
    assertion_id   TEXT PRIMARY KEY,
    kind           TEXT NOT NULL CHECK (kind IN ('authority','not-authority')),
    source_id      TEXT NOT NULL,
    locator        TEXT NOT NULL,
    actor_kind     TEXT NOT NULL CHECK (actor_kind = 'user'),
    actor_id       TEXT NOT NULL,
    reason         TEXT NOT NULL,
    -- What the person was looking at: the subject's latest observation.
    evidence       TEXT[] NOT NULL,
    asserted_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    retracted_at   TIMESTAMPTZ,
    retracted_by   TEXT,
    retract_reason TEXT,
    CONSTRAINT retraction_complete CHECK (
        (retracted_at IS NULL) = (retracted_by IS NULL)
        AND (retracted_at IS NULL) = (retract_reason IS NULL))
);

CREATE INDEX IF NOT EXISTS assertions_subject_idx ON assertions (source_id, locator);

-- Enforced here rather than trusted to application code, as with
-- observations. The only permitted update is a retraction of an
-- unretracted row.
CREATE OR REPLACE FUNCTION assertions_are_durable() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'assertions are never deleted: retract % instead', OLD.assertion_id;
    END IF;
    IF OLD.retracted_at IS NOT NULL THEN
        RAISE EXCEPTION 'assertion % is already retracted', OLD.assertion_id;
    END IF;
    IF NEW.kind <> OLD.kind OR NEW.source_id <> OLD.source_id OR NEW.locator <> OLD.locator
       OR NEW.actor_kind <> OLD.actor_kind OR NEW.actor_id <> OLD.actor_id
       OR NEW.reason <> OLD.reason OR NEW.evidence <> OLD.evidence
       OR NEW.asserted_at <> OLD.asserted_at THEN
        RAISE EXCEPTION 'assertions are never edited: only a retraction may be recorded on %', OLD.assertion_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS assertions_durable ON assertions;
CREATE TRIGGER assertions_durable
    BEFORE UPDATE OR DELETE ON assertions
    FOR EACH ROW EXECUTE FUNCTION assertions_are_durable();

-- The authority state of every present file, rebuilt on each pass.
CREATE TABLE IF NOT EXISTS file_authority (
    source_id         TEXT NOT NULL,
    locator           TEXT NOT NULL,
    -- declared | likely | multiple | none
    state             TEXT NOT NULL CHECK (state IN ('declared','likely','multiple','none')),
    -- explicit | identity-profile | '' when there is no authority to point at
    basis             TEXT NOT NULL,
    authority_source  TEXT,
    authority_locator TEXT,
    confidence        REAL NOT NULL,
    evidence          TEXT[] NOT NULL,
    assertion_id      TEXT,
    explanation       TEXT NOT NULL,
    PRIMARY KEY (source_id, locator)
);

COMMIT;
