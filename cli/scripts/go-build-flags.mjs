/**
 * Shared Go build configuration for the public `loaf` command.
 *
 * build-go.mjs compiles every local and release binary through goBuildArgs, so
 * every distribution channel is built the same way. Keeping the flags here is
 * the single source of truth.
 *
 * Release metadata (commit + date) is injected via `-X main.buildCommit/buildDate`
 * only when LOAF_BUILD_COMMIT / LOAF_BUILD_DATE are present in the environment.
 * Only the release workflow sets them, so their absence is half of what marks a
 * binary as a dev build — the other half is running out of a source checkout
 * (see internal/cli/version.go).
 *
 * `-buildvcs=true` makes the toolchain stamp the source commit (`vcs.revision`)
 * and working-tree state (`vcs.modified`) into the binary. A dev build reads
 * that stamp back and reports `<package-version>+g<short-sha>`, adding `.dirty`
 * when the tree had uncommitted changes at compile time. The identity is part
 * of the compiled bytes, so it cannot drift from them. Outside any checkout the
 * toolchain writes no stamp and the binary reports the bare package version;
 * inside a checkout whose Git cannot be queried, `true` (unlike `auto`) fails
 * the build instead of silently dropping provenance.
 *
 * Linked worktrees (a `.git` file, not a directory) are not stamped by every
 * toolchain: go1.26.6, the version go.mod pins, writes nothing there, while
 * go1.27.1 does. build-go.mjs therefore resolves the same two facts with git
 * and passes them as LOAF_DEV_COMMIT / LOAF_DEV_MODIFIED, which link in as
 * `-X main.devCommit/devModified`. The runtime prefers the toolchain stamp and
 * falls back to the linked values, so both paths report one identity. Once the
 * pinned toolchain stamps worktrees, the fallback can go.
 */

export function goLdflags(env = process.env) {
  const parts = ["-buildid="];
  const commit = (env.LOAF_BUILD_COMMIT || "").trim();
  const date = (env.LOAF_BUILD_DATE || "").trim();
  if (commit) {
    parts.push(`-X main.buildCommit=${commit}`);
  }
  if (date) {
    parts.push(`-X main.buildDate=${date}`);
  }
  const devCommit = (env.LOAF_DEV_COMMIT || "").trim();
  if (devCommit) {
    parts.push(`-X main.devCommit=${devCommit}`);
    parts.push(`-X main.devModified=${env.LOAF_DEV_MODIFIED === "1" ? "true" : "false"}`);
  }
  return parts.join(" ");
}

export function goBuildArgs(output, env = process.env) {
  return ["build", "-trimpath", "-buildvcs=true", "-ldflags", goLdflags(env), "-o", output, "./cmd/loaf"];
}
