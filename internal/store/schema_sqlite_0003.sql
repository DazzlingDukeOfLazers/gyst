ALTER TABLE assertions RENAME TO assertions_old;
CREATE TABLE assertions (
    assertion_id   TEXT PRIMARY KEY,
    kind           TEXT NOT NULL CHECK (kind IN ('authority','not-authority','project.confirm','project.ignore')),
    subject_kind   TEXT NOT NULL DEFAULT 'file' CHECK (subject_kind IN ('file','project')),
    source_id      TEXT NOT NULL,
    locator        TEXT NOT NULL,
    object         TEXT NOT NULL DEFAULT '',
    value          TEXT NOT NULL DEFAULT '',
    actor_kind     TEXT NOT NULL CHECK (actor_kind = 'user'),
    actor_id       TEXT NOT NULL,
    reason         TEXT NOT NULL,
    evidence       TEXT NOT NULL,
    asserted_at    TEXT NOT NULL,
    retracted_at   TEXT,
    retracted_by   TEXT,
    retract_reason TEXT,
    CONSTRAINT retraction_complete CHECK (
        (retracted_at IS NULL) = (retracted_by IS NULL)
        AND (retracted_at IS NULL) = (retract_reason IS NULL))
);
INSERT INTO assertions (assertion_id, kind, subject_kind, source_id, locator, object, value, actor_kind, actor_id, reason, evidence, asserted_at, retracted_at, retracted_by, retract_reason)
    SELECT assertion_id, kind, 'file', source_id, locator, '', '', actor_kind, actor_id, reason, evidence, asserted_at, retracted_at, retracted_by, retract_reason FROM assertions_old;
DROP TABLE assertions_old;
CREATE INDEX IF NOT EXISTS assertions_subject_idx ON assertions (source_id, locator);
CREATE TRIGGER IF NOT EXISTS assertions_no_delete BEFORE DELETE ON assertions
BEGIN SELECT RAISE(ABORT, 'assertions are never deleted: retract instead'); END;
CREATE TRIGGER IF NOT EXISTS assertions_no_edit BEFORE UPDATE ON assertions
WHEN OLD.retracted_at IS NOT NULL
  OR NEW.kind <> OLD.kind OR NEW.subject_kind <> OLD.subject_kind
  OR NEW.source_id <> OLD.source_id OR NEW.locator <> OLD.locator
  OR NEW.object <> OLD.object OR NEW.value <> OLD.value
  OR NEW.actor_kind <> OLD.actor_kind OR NEW.actor_id <> OLD.actor_id
  OR NEW.reason <> OLD.reason OR NEW.evidence <> OLD.evidence OR NEW.asserted_at <> OLD.asserted_at
BEGIN SELECT RAISE(ABORT, 'assertions are never edited: only a retraction may be recorded'); END;
ALTER TABLE projects ADD COLUMN state TEXT NOT NULL DEFAULT 'candidate' CHECK (state IN ('declared','candidate','confirmed','ignored'));
