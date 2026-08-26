DROP INDEX IF EXISTS idx_verdicts_run;
DROP INDEX IF EXISTS idx_verdicts_finding;
DROP INDEX IF EXISTS idx_findings_filter;
DROP INDEX IF EXISTS idx_findings_run;
DROP INDEX IF EXISTS idx_findings_report;
DROP INDEX IF EXISTS idx_runs_generator;

DROP TABLE IF EXISTS verdicts;
DROP TABLE IF EXISTS findings;
DROP TABLE IF EXISTS runs;
