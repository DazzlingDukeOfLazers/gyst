-- Confidence as double precision.
--
-- The confidence columns were REAL, and a REAL read into a float64 is
-- 0.6000000238418579 where 0.6 was written. Postgres hid this wherever a
-- value passed through json_agg, which prints float4 with six digits, and
-- showed it wherever a value was scanned directly. Drawing the store
-- boundary (ADR 004) made the two paths one path and the artefact visible.
-- Double precision is what every reader expects and what SQLite's REAL is,
-- so the engines agree by construction. Existing values are rounded on
-- conversion; nothing legitimately carries more than six decimals.

BEGIN;

ALTER TABLE artifacts        ALTER COLUMN confidence TYPE DOUBLE PRECISION USING round(confidence::numeric, 6)::double precision;
ALTER TABLE artifact_members ALTER COLUMN confidence TYPE DOUBLE PRECISION USING round(confidence::numeric, 6)::double precision;
ALTER TABLE relations        ALTER COLUMN confidence TYPE DOUBLE PRECISION USING round(confidence::numeric, 6)::double precision;
ALTER TABLE projects         ALTER COLUMN confidence TYPE DOUBLE PRECISION USING round(confidence::numeric, 6)::double precision;
ALTER TABLE project_members  ALTER COLUMN confidence TYPE DOUBLE PRECISION USING round(confidence::numeric, 6)::double precision;
ALTER TABLE file_projects    ALTER COLUMN confidence TYPE DOUBLE PRECISION USING round(confidence::numeric, 6)::double precision;
ALTER TABLE findings         ALTER COLUMN confidence TYPE DOUBLE PRECISION USING round(confidence::numeric, 6)::double precision;
ALTER TABLE file_authority   ALTER COLUMN confidence TYPE DOUBLE PRECISION USING round(confidence::numeric, 6)::double precision;
ALTER TABLE sources          ALTER COLUMN location_confidence TYPE DOUBLE PRECISION USING round(location_confidence::numeric, 6)::double precision;

COMMIT;
