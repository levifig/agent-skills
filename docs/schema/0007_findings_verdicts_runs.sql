CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  generator_ref TEXT NOT NULL,
  generator_version TEXT,
  generator_hash TEXT,
  status TEXT NOT NULL,
  metadata TEXT,
  started_at TEXT,
  completed_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS findings (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  report_id TEXT NOT NULL,
  run_id TEXT,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  severity TEXT NOT NULL,
  confidence TEXT NOT NULL,
  dimension TEXT,
  path TEXT,
  line_start INTEGER,
  line_end INTEGER,
  symbol TEXT,
  metadata TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (report_id) REFERENCES reports(id),
  FOREIGN KEY (run_id) REFERENCES runs(id)
);

CREATE TABLE IF NOT EXISTS verdicts (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  finding_id TEXT NOT NULL,
  run_id TEXT,
  outcome TEXT NOT NULL,
  rationale TEXT NOT NULL,
  reproduction_notes TEXT,
  metadata TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (finding_id) REFERENCES findings(id),
  FOREIGN KEY (run_id) REFERENCES runs(id)
);

CREATE INDEX IF NOT EXISTS idx_runs_generator ON runs (project_id, generator_ref, status);
CREATE INDEX IF NOT EXISTS idx_findings_report ON findings (project_id, report_id);
CREATE INDEX IF NOT EXISTS idx_findings_run ON findings (project_id, run_id);
CREATE INDEX IF NOT EXISTS idx_findings_filter ON findings (project_id, severity, status, confidence);
CREATE INDEX IF NOT EXISTS idx_verdicts_finding ON verdicts (project_id, finding_id);
CREATE INDEX IF NOT EXISTS idx_verdicts_run ON verdicts (project_id, run_id);
