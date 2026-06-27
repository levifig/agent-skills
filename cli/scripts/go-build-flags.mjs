/**
 * Shared Go build configuration for the public `loaf` command.
 *
 * Both build-go.mjs (produces the committed/release binaries) and
 * verify-go-artifacts.mjs (reproducibility check) must use identical `go build`
 * arguments, or the byte-for-byte reproducibility assertion breaks. Keeping the
 * ldflags here is the single source of truth.
 *
 * Build metadata (commit + date) is injected via `-X main.buildCommit/buildDate`
 * only when LOAF_BUILD_COMMIT / LOAF_BUILD_DATE are present in the environment.
 * Without them the binary is the clean, deterministic default — `loaf --version`
 * shows just the semver, and local/CI builds stay reproducible.
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
  return parts.join(" ");
}

export function goBuildArgs(output, env = process.env) {
  return ["build", "-trimpath", "-buildvcs=false", "-ldflags", goLdflags(env), "-o", output, "./cmd/loaf"];
}
