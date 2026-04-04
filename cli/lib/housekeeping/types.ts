/**
 * Types for the cleanup scanner.
 */

export type ArtifactType =
  | "session"
  | "task"
  | "spec"
  | "plan"
  | "draft"
  | "council"
  | "report";

export const ARTIFACT_TYPES: ArtifactType[] = [
  "session",
  "task",
  "spec",
  "plan",
  "draft",
  "council",
  "report",
];

export type RecommendedAction = "archive" | "delete" | "flag" | "skip";

export interface CleanupRecommendation {
  type: ArtifactType;
  path: string;
  filename: string;
  action: RecommendedAction;
  reason: string;
  hint?: string;
  frontmatter?: Record<string, unknown>;
}

export interface TypeSummary {
  type: ArtifactType;
  total: number;
  archive: number;
  delete: number;
  flag: number;
  skip: number;
}

export interface ScanResult {
  recommendations: CleanupRecommendation[];
  summary: TypeSummary[];
  warnings: string[];
}

export interface ScanOptions {
  agentsDir: string;
  filter?: ArtifactType[];
}

export interface ArtifactDirectory {
  type: ArtifactType;
  dirname: string;
  required: boolean;
}

export const ARTIFACT_DIRS: ArtifactDirectory[] = [
  { type: "session", dirname: "sessions", required: true },
  { type: "task", dirname: "tasks", required: true },
  { type: "spec", dirname: "specs", required: true },
  { type: "plan", dirname: "plans", required: false },
  { type: "draft", dirname: "drafts", required: false },
  { type: "council", dirname: "councils", required: false },
  { type: "report", dirname: "reports", required: false },
];
