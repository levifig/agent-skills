import { runBuild } from "./commands/build.js";

const args = process.argv.slice(2);
let target: string | undefined;

for (let i = 0; i < args.length; i++) {
  const arg = args[i];
  if (arg === "-t" || arg === "--target") {
    target = args[i + 1];
    i++;
    continue;
  }
  if (arg.startsWith("--target=")) {
    target = arg.slice("--target=".length);
    continue;
  }
  throw new Error(`Unknown argument: ${arg}`);
}

try {
  await runBuild({ target });
} catch (error) {
  const message = error instanceof Error ? error.message : String(error);
  console.error(message);
  process.exit(1);
}
