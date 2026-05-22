import { Command } from "commander";
import { registerBuildCommand } from "./commands/build.js";
import { registerInstallCommand } from "./commands/install.js";
import { registerInitCommand } from "./commands/init.js";
import { registerReleaseCommand } from "./commands/release.js";
import { registerTaskCommand } from "./commands/task.js";
import { registerSpecCommand } from "./commands/spec.js";
import { registerKbCommand } from "./commands/kb.js";
import { registerSetupCommand } from "./commands/setup.js";
import { registerVersionCommand } from "./commands/version.js";
import { registerHousekeepingCommand } from "./commands/housekeeping.js";
import { registerCheckCommand } from "./commands/check.js";
import { registerDoctorCommand } from "./commands/doctor.js";
import { registerSessionCommand } from "./commands/session.js";
import { registerReportCommand } from "./commands/report.js";
import { registerMigrateCommand } from "./commands/migrate.js";
import { LOAF_VERSION } from "./lib/version.js";
import {
  detectMainMissingForRefusal,
  detectPreA3State,
  PRE_A3_REFUSAL_MESSAGE,
} from "./lib/migrate/worktree-storage.js";

const program = new Command();

program.name("loaf").description("Loaf — An Opinionated Agentic Framework").version(LOAF_VERSION, "-v, --version");

// Register all commands first
registerBuildCommand(program);
registerInstallCommand(program);
registerInitCommand(program);
registerReleaseCommand(program);
registerTaskCommand(program);
registerSpecCommand(program);
registerKbCommand(program);
registerSetupCommand(program);
registerVersionCommand(program);
registerHousekeepingCommand(program);
registerCheckCommand(program);
registerDoctorCommand(program);
registerSessionCommand(program);
registerReportCommand(program);
registerMigrateCommand(program);

// Check for --agent-help before parsing (needs commands registered first)
if (process.argv.includes("--agent-help")) {
  const { generateCliJson } = await import("./lib/cli-reference-generator.js");
  console.log(generateCliJson(program));
  process.exit(0);
}

// Show help when no subcommand is given (exit 0, not error)
if (process.argv.length <= 2) {
  program.outputHelp();
  process.exit(0);
}

// SPEC-036 / TASK-170 — Pre-A3 refusal nudge.
//
// Before any subcommand runs, refuse every command except `migrate` (and
// help/version flags) when the current worktree is in pre-A3 state:
//   - inside a linked git worktree, AND
//   - worktree-local `.agents/` contains content other than `.moved-to`, AND
//   - back-pointer file is absent OR doesn't point to the current main root.
//
// Single-checkout repos and main worktrees never trigger this (the detector
// short-circuits on the cheapest signal first).
//
// When the refusal would fire BUT the main worktree's target path no longer
// exists (or is not a directory), telling the user to run `loaf migrate
// worktree-storage` is cheerful misdirection — the migrate command itself
// can't complete because its target is gone. Surface the actual problem
// instead.
if (shouldRefuseCommand(process.argv)) {
  const unknown = unknownTopLevelCommand(process.argv);
  if (unknown) {
    process.stderr.write(`error: unknown command '${unknown}'\n\n`);
  }
  const mainMissing = detectMainMissingForRefusal(process.cwd());
  if (mainMissing) {
    process.stderr.write(`${mainMissing}\n`);
  } else {
    process.stderr.write(`${PRE_A3_REFUSAL_MESSAGE}\n`);
  }
  process.exit(2);
}

program.parse();

/**
 * Decide whether to refuse the invocation based on the argv and the
 * pre-A3 detector. Exempts: `migrate` (and its sub-subcommands), and the
 * help/version flags so the user can always discover the migration command.
 */
function shouldRefuseCommand(argv: string[]): boolean {
  // Inspect the first positional arg after `node <script>`.
  const args = argv.slice(2);
  if (args.length === 0) return false;

  // Allow help/version flags anywhere on the line.
  for (const a of args) {
    if (a === "--help" || a === "-h" || a === "--version" || a === "-v") {
      return false;
    }
  }

  const sub = args[0];
  if (sub === "migrate") return false;
  // Allow `loaf help <cmd>` (Commander's built-in help facility) so users
  // in pre-A3 state can run `loaf help migrate` to learn about the command.
  if (sub === "help") return false;

  return detectPreA3State(process.cwd());
}

function unknownTopLevelCommand(argv: string[]): string | null {
  const sub = argv.slice(2)[0];
  if (!sub || sub.startsWith("-")) return null;
  if (sub === "help") return null;

  const known = program.commands.some((cmd) => {
    if (cmd.name() === sub) return true;
    return cmd.aliases().includes(sub);
  });
  return known ? null : sub;
}
