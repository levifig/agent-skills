/**
 * Version Injection Utility
 *
 * Reads version from package.json and injects it into frontmatter at build time.
 * This ensures all generated files have consistent version metadata.
 */

import { readFileSync } from "fs";
import { join } from "path";

/**
 * Get the current package version from package.json
 *
 * @param {string} rootDir - Path to repository root
 * @returns {string} Version string (e.g., "1.9.0")
 */
export function getVersion(rootDir) {
  const pkgPath = join(rootDir, "package.json");
  const pkg = JSON.parse(readFileSync(pkgPath, "utf-8"));
  return pkg.version;
}

/**
 * Inject version into frontmatter object
 *
 * @param {Object} frontmatter - Existing frontmatter
 * @param {string} version - Version string to inject
 * @returns {Object} New frontmatter with version added
 */
export function injectVersion(frontmatter, version) {
  return { ...frontmatter, version };
}
