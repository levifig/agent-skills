/**
 * loaf migrate worktree-storage — SPEC-036 / TASK-170
 *
 * Thin Commander adapter around `runMigration` in
 * `cli/lib/migrate/worktree-storage.ts`. Keeps process.exit and CLI parsing
 * out of the migration logic so the migration is unit-testable.
 *
 * Surface:
 *   loaf migrate worktree-storage           # dry-run preview (default)
 *   loaf migrate worktree-storage --apply   # perform the migration
 *
 * Conflict overrides (mutually exclusive — Commander does not enforce this,
 * the action handler does):
 *   --force-from-worktree   keep the worktree-local copy on every conflict
 *   --force-from-main       keep the main-worktree copy on every conflict
 *   (default)               newest mtime wins
 *
 * Exit codes:
 *   0 — dry-run completed, migration applied successfully, or nothing to do
 *   1 — error (not in a git repository, mutually-exclusive flags, IO failure)
 */

import { Command } from "commander";

import {
  DEBUG_RESOLVE_ENV,
} from "../lib/tasks/resolve.js";
import {
  formatResult,
  runMigration,
  type ConflictResolution,
} from "../lib/migrate/worktree-storage.js";

const red = (s: string) => `\x1b[31m${s}\x1b[0m`;

interface MigrateWorktreeStorageOptions {
  apply?: boolean;
  forceFromWorktree?: boolean;
  forceFromMain?: boolean;
}

export function registerMigrateCommand(program: Command): void {
  const migrate = program
    .command("migrate")
    .description("One-shot migrations for Loaf state and storage layout");

  migrate
    .command("worktree-storage")
    .description(
      "Move a linked worktree's local .agents/ into the main worktree (SPEC-036). " +
      "Dry-run by default; pass --apply to mutate. " +
      "Invoking from the main checkout is a clean no-op. " +
      `Set ${DEBUG_RESOLVE_ENV}=1 to surface git probe diagnostics if the migration ` +
      "behaves unexpectedly.",
    )
    .option("--apply", "Perform the migration (default: dry-run preview only)")
    .option(
      "--force-from-worktree",
      "On conflict, always keep the worktree-local copy",
    )
    .option(
      "--force-from-main",
      "On conflict, always keep the main-worktree copy",
    )
    .action((options: MigrateWorktreeStorageOptions) => {
      if (options.forceFromWorktree && options.forceFromMain) {
        console.error(
          `${red("error:")} --force-from-worktree and --force-from-main are mutually exclusive`,
        );
        process.exit(1);
      }

      const conflictPolicy: ConflictResolution = options.forceFromWorktree
        ? "worktree"
        : options.forceFromMain
          ? "main"
          : "newer";

      try {
        const result = runMigration({
          cwd: process.cwd(),
          apply: options.apply ?? false,
          conflictPolicy,
        });

        const output = formatResult(result, { apply: options.apply ?? false });

        if (result.status === "not-in-git") {
          console.error(`${red("error:")} ${result.message}`);
          process.exit(1);
        }

        if (result.status === "main-missing") {
          // The main worktree's path resolved but does not exist on disk (or
          // is not a directory). Surface the full diagnostic and exit
          // non-zero — there's no safe action we can take here.
          console.error(`${red("error:")} ${result.message}`);
          process.exit(1);
        }

        if (result.status === "partial-leftover") {
          // Refusal: a previous run was interrupted mid-EXDEV-stage. Print
          // the formatted output (it lists the leftover paths) to stderr
          // and exit non-zero so the user notices.
          console.error(`${red("error:")} ${result.message}`);
          process.exit(1);
        }

        console.log(output);
        process.exit(0);
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        console.error(`${red("error:")} migration failed: ${msg}`);
        process.exit(1);
      }
    });
}
