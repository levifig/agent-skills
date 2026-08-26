-- Work contracts: machinery keyed to provider-qualified authority refs.
--
-- v1 providers: linear, branch, pr. Unsupported provider kinds refuse at the
-- API layer and never fall back to internal issue rows. Legacy issue rows
-- remain until LOAF-83 migrates them; this migration introduces parallel
-- ref-keyed storage only.

CREATE TABLE IF NOT EXISTS work_contracts (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  provider TEXT NOT NULL CHECK (provider IN ('linear', 'branch', 'pr')),
  provider_ref TEXT NOT NULL CHECK (length(trim(provider_ref)) > 0),
  kind TEXT NOT NULL CHECK (kind IN ('delivery', 'decision')),
  title TEXT NOT NULL CHECK (length(trim(title)) > 0),
  body TEXT NOT NULL DEFAULT '',
  fog TEXT,
  status TEXT NOT NULL CHECK (status IN ('triage', 'backlog', 'todo', 'active', 'done', 'cancelled', 'duplicate')),
  parent_contract_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, parent_contract_id) REFERENCES work_contracts(project_id, id),
  UNIQUE (project_id, provider, provider_ref),
  UNIQUE (project_id, id)
);
CREATE INDEX IF NOT EXISTS idx_work_contracts_project ON work_contracts (project_id, created_at);
CREATE INDEX IF NOT EXISTS idx_work_contracts_parent ON work_contracts (project_id, parent_contract_id);

CREATE TABLE IF NOT EXISTS work_contract_criteria (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  contract_id TEXT NOT NULL,
  position INTEGER NOT NULL CHECK (position >= 1),
  text TEXT NOT NULL CHECK (length(trim(text)) > 0),
  command TEXT,
  expect TEXT,
  tier TEXT NOT NULL CHECK (tier IN ('V', 'H')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, contract_id) REFERENCES work_contracts(project_id, id) ON DELETE CASCADE,
  UNIQUE (contract_id, position)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_work_contract_criteria_project_id ON work_contract_criteria (project_id, id);
CREATE INDEX IF NOT EXISTS idx_work_contract_criteria_contract ON work_contract_criteria (project_id, contract_id, position);

CREATE TABLE IF NOT EXISTS work_contract_criterion_claims (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  child_criterion_id TEXT NOT NULL,
  parent_criterion_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, child_criterion_id) REFERENCES work_contract_criteria(project_id, id) ON DELETE CASCADE,
  FOREIGN KEY (project_id, parent_criterion_id) REFERENCES work_contract_criteria(project_id, id) ON DELETE CASCADE,
  UNIQUE (child_criterion_id, parent_criterion_id),
  CHECK (child_criterion_id != parent_criterion_id)
);
CREATE INDEX IF NOT EXISTS idx_work_contract_criterion_claims_parent ON work_contract_criterion_claims (project_id, parent_criterion_id);
CREATE INDEX IF NOT EXISTS idx_work_contract_criterion_claims_child ON work_contract_criterion_claims (project_id, child_criterion_id);

CREATE TABLE IF NOT EXISTS work_contract_workspace (
  contract_id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  started_branch TEXT,
  started_worktree TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, contract_id) REFERENCES work_contracts(project_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS work_contract_mappings (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  provider TEXT NOT NULL CHECK (provider IN ('linear', 'branch', 'pr')),
  provider_ref TEXT NOT NULL CHECK (length(trim(provider_ref)) > 0),
  mapping_kind TEXT NOT NULL CHECK (length(trim(mapping_kind)) > 0),
  mapping_value TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  UNIQUE (project_id, provider, provider_ref, mapping_kind)
);
CREATE INDEX IF NOT EXISTS idx_work_contract_mappings_ref ON work_contract_mappings (project_id, provider, provider_ref);

CREATE TABLE IF NOT EXISTS work_contract_receipts (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  provider TEXT NOT NULL CHECK (provider IN ('linear', 'branch', 'pr')),
  provider_ref TEXT NOT NULL CHECK (length(trim(provider_ref)) > 0),
  receipt_kind TEXT NOT NULL CHECK (length(trim(receipt_kind)) > 0),
  receipt_value TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  UNIQUE (project_id, provider, provider_ref, receipt_kind)
);
CREATE INDEX IF NOT EXISTS idx_work_contract_receipts_ref ON work_contract_receipts (project_id, provider, provider_ref);
