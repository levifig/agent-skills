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
 * Only the release workflow sets them, so their absence is half of what marks a
 * binary as a dev build — the other half is running out of a source checkout,
 * since the binaries this script commits also ship as releases through the
 * plugin marketplace (see internal/cli/version.go).
 *
 * A dev build reports `<package-version>+g<short-sha>`. build-go.mjs records the
 * SHA in an ignored provenance file beside bin/native only after every requested
 * target compiles, rather than injecting it here, so verify-go-artifacts.mjs can
 * still reproduce the tracked binaries byte for byte and shipped distributions
 * do not inherit local provenance.
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
