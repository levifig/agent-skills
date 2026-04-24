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
import { LOAF_VERSION } from "./lib/version.js";

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

program.parse();
