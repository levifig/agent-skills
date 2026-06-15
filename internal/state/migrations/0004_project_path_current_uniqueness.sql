CREATE UNIQUE INDEX IF NOT EXISTS idx_project_paths_one_current ON project_paths (project_id) WHERE is_current = 1;
