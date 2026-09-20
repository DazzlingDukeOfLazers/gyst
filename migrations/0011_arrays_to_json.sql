-- Array columns become JSON arrays.
--
-- TEXT[] is PostgreSQL's own. Every reader of these columns already treats
-- them as lists of strings, the report emits them as JSON arrays, and
-- SQLite has JSON and no arrays. Storing them as JSON in both engines
-- means one spelling for the write, one for the read, and a constraint
-- that says the same thing in each dialect. ADR 004 stage 2.

BEGIN;

ALTER TABLE relations DROP CONSTRAINT IF EXISTS relation_cites_evidence;
ALTER TABLE relations      ALTER COLUMN evidence TYPE JSONB USING to_jsonb(evidence);
ALTER TABLE relations ADD CONSTRAINT relation_cites_evidence CHECK (jsonb_array_length(evidence) >= 1);

ALTER TABLE findings       ALTER COLUMN evidence TYPE JSONB USING to_jsonb(evidence);
ALTER TABLE assertions     ALTER COLUMN evidence TYPE JSONB USING to_jsonb(evidence);
ALTER TABLE file_authority ALTER COLUMN evidence TYPE JSONB USING to_jsonb(evidence);
ALTER TABLE projects       ALTER COLUMN evidence TYPE JSONB USING to_jsonb(evidence);
ALTER TABLE commits        ALTER COLUMN parents  DROP DEFAULT;
ALTER TABLE commits        ALTER COLUMN parents  TYPE JSONB USING to_jsonb(parents);
ALTER TABLE commits        ALTER COLUMN parents  SET DEFAULT '[]'::jsonb;

COMMIT;
