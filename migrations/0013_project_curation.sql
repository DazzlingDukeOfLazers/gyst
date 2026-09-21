-- Project curation assertions: a person confirms a candidate as a project
-- or sets it aside, durably and retractably (design: project inventory
-- curation, sections 1.1 and 14; handoff answer to 10.1).
--
-- The assertions table already holds a person's word about a file with
-- actor, reason, evidence, time, and retraction, guarded by a trigger. It
-- gains a subject kind, so a subject can be a project record, and two
-- kinds. For a project subject, locator carries the project id and
-- source_id is empty. object and value are reserved for the kinds that
-- follow (containment, alias, membership) and are empty here.

BEGIN;

ALTER TABLE assertions DROP CONSTRAINT IF EXISTS assertions_kind_check;
ALTER TABLE assertions ADD CONSTRAINT assertions_kind_check
    CHECK (kind IN ('authority','not-authority','project.confirm','project.ignore'));
ALTER TABLE assertions
    ADD COLUMN IF NOT EXISTS subject_kind TEXT NOT NULL DEFAULT 'file',
    ADD COLUMN IF NOT EXISTS object       TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS value        TEXT NOT NULL DEFAULT '';
ALTER TABLE assertions DROP CONSTRAINT IF EXISTS assertions_subject_kind_check;
ALTER TABLE assertions ADD CONSTRAINT assertions_subject_kind_check CHECK (subject_kind IN ('file','project'));

CREATE OR REPLACE FUNCTION assertions_are_durable() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'assertions are never deleted: retract % instead', OLD.assertion_id;
    END IF;
    IF OLD.retracted_at IS NOT NULL THEN
        RAISE EXCEPTION 'assertion % is already retracted', OLD.assertion_id;
    END IF;
    IF NEW.kind <> OLD.kind OR NEW.subject_kind <> OLD.subject_kind
       OR NEW.source_id <> OLD.source_id OR NEW.locator <> OLD.locator
       OR NEW.object <> OLD.object OR NEW.value <> OLD.value
       OR NEW.actor_kind <> OLD.actor_kind OR NEW.actor_id <> OLD.actor_id
       OR NEW.reason <> OLD.reason OR NEW.evidence <> OLD.evidence
       OR NEW.asserted_at <> OLD.asserted_at THEN
        RAISE EXCEPTION 'assertions are never edited: only a retraction may be recorded on %', OLD.assertion_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- A project record's review state. declared: a manifest says so.
-- candidate: a marker suggests it and nobody has judged. confirmed and
-- ignored: a person judged, by assertion.
ALTER TABLE projects ADD COLUMN IF NOT EXISTS state TEXT NOT NULL DEFAULT 'candidate'
    CHECK (state IN ('declared','candidate','confirmed','ignored'));

COMMIT;
