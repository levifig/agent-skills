-- Drop fossil polymorphic references to finding/verdict/run kinds before
-- dropping the tables. Leftover relationships/aliases naming those kinds would
-- break trace/link hydrate once entityDetails no longer recognizes them.
DELETE FROM relationships
WHERE from_entity_kind IN ('finding', 'verdict', 'run')
   OR to_entity_kind IN ('finding', 'verdict', 'run');

DELETE FROM aliases
WHERE entity_kind IN ('finding', 'verdict', 'run');

DELETE FROM events
WHERE entity_kind IN ('finding', 'verdict', 'run');

DELETE FROM entity_tags
WHERE entity_kind IN ('finding', 'verdict', 'run');

DELETE FROM bundle_members
WHERE entity_kind IN ('finding', 'verdict', 'run');

DELETE FROM backend_mappings
WHERE entity_kind IN ('finding', 'verdict', 'run');

DELETE FROM exports
WHERE source_entity_kind IN ('finding', 'verdict', 'run');

DELETE FROM artifact_bodies
WHERE entity_kind IN ('finding', 'verdict', 'run');

DROP INDEX IF EXISTS idx_verdicts_run;
DROP INDEX IF EXISTS idx_verdicts_finding;
DROP INDEX IF EXISTS idx_findings_filter;
DROP INDEX IF EXISTS idx_findings_run;
DROP INDEX IF EXISTS idx_findings_report;
DROP INDEX IF EXISTS idx_runs_generator;

DROP TABLE IF EXISTS verdicts;
DROP TABLE IF EXISTS findings;
DROP TABLE IF EXISTS runs;
