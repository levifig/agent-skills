DELETE FROM artifact_bodies WHERE entity_kind IN ('report', 'council', 'shaping_draft');
DELETE FROM aliases WHERE entity_kind IN ('report', 'council', 'shaping_draft');
DELETE FROM events WHERE entity_kind IN ('report', 'council', 'shaping_draft');
DELETE FROM entity_tags WHERE entity_kind IN ('report', 'council', 'shaping_draft');
DELETE FROM relationships WHERE from_entity_kind IN ('report', 'council', 'shaping_draft') OR to_entity_kind IN ('report', 'council', 'shaping_draft');

DROP INDEX IF EXISTS idx_councils_spec;

DROP TABLE IF EXISTS councils;
DROP TABLE IF EXISTS shaping_drafts;
DROP TABLE IF EXISTS reports;
