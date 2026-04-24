/**
 * Loaf version resolution.
 *
 * __LOAF_VERSION__ is injected at build time by tsup's `define` config.
 * In an unbuilt/dev/test context where the define isn't applied, fall
 * back to reading package.json so `npm run test` and `tsx` still work.
 */

import { readFileSync } from "fs";
import { dirname, join } from "path";
import { fileURLToPath } from "url";

declare const __LOAF_VERSION__: string | undefined;

function readPackageVersion(): string {
  try {
    const here = dirname(fileURLToPath(import.meta.url));
    // Walk up from cli/lib/ to find the repo root package.json
    for (const candidate of [
      join(here, "..", "..", "package.json"),       // from dist-cli/ (bundled)
      join(here, "..", "..", "..", "package.json"),  // from cli/lib/ (source)
      join(here, "..", "..", "..", "..", "package.json"), // extra safety
    ]) {
      try {
        const pkg = JSON.parse(readFileSync(candidate, "utf-8"));
        if (pkg.name === "loaf") return pkg.version;
      } catch {
        continue;
      }
    }
  } catch {
    /* noop */
  }
  return "0.0.0";
}

export const LOAF_VERSION: string =
  typeof __LOAF_VERSION__ !== "undefined" && __LOAF_VERSION__
    ? __LOAF_VERSION__
    : readPackageVersion();
