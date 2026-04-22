import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    root: ".",
    include: ["cli/**/*.test.ts"],
    // Several CLI tests spawn subprocesses that share the repo working tree
    // (fixtures, .agents/ state). Running files in parallel produces flaky
    // cross-file pollution. Serializing files is ~20% slower but deterministic.
    // Follow-up: migrate remaining CWD-relative fixtures to mkdtempSync and
    // re-enable parallelism.
    fileParallelism: false,
  },
});
