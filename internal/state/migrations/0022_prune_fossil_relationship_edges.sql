-- Prune polymorphic relationship edges that name vocabulary-fossil entity kinds.
-- Fossil rows and aliases stay in local-archive tables; only sync-facing edges
-- that would cross the minimal core boundary are removed.
DELETE FROM relationships
WHERE from_entity_kind IN ('plan', 'spec', 'task', 'intent', 'brainstorm')
   OR to_entity_kind IN ('plan', 'spec', 'task', 'intent', 'brainstorm');
