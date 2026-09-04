# Amp Native Delegation

## Adapter Contract

The managed Amp plugin exposes `loaf_delegate` for one bounded implementation or review turn against a native tracker contract. This is an Amp-specific local adapter, not a generic delegation framework.

- The main agent reads the live tracker record and supplies its native reference and bounded packet. The child does not read or mutate the provider.
- The caller supplies the current canonical local Git worktree root. The adapter verifies it against both Amp's workspace root and `git rev-parse --show-toplevel` before creating the child.
- The child inherits the parent's current built-in Amp mode. Custom agent definitions, including ones with model or effort overrides, fail preflight rather than silently falling back.
- Execution uses Amp's local executor only. Amp Orbs and other remote runners are not supported.
- Extending the native mode preserves normal project context; limiting tools does not remove that context.

## Implementation Boundary

An implementation child receives only `Read` and `apply_patch`. It has no shell, provider, or delegation tools. Read paths and every `apply_patch` file header and `Move to` header must name an absolute path. The adapter rejects paths outside the canonical worktree, paths through escaping symlinks, and Git metadata paths.

The caller coordinates one writer and performs no concurrent parent writes. A plugin-runtime writer guard rejects another implementation child while ownership is active or uncertain, and the child's guard is sealed after its turn.

This is a trusted-host, single-plugin-runtime guard. It is not an OS sandbox or a cross-process lock, so coordination outside that runtime remains the caller's responsibility.

If the parent becomes idle or errors while delegation is pending, the adapter seals the child and requests cancellation. An unresolved submission or unconfirmed stop returns `uncertain` and retains writer ownership. Inspect the returned child before restarting Amp or attempting other writes; a cancellation request is not proof the child stopped. Plugin crashes and reloads lose these in-memory guards.

## Review and Acceptance

A reviewer receives zero tools and reviews only a caller-prepared, complete immutable snapshot containing the exact diff, relevant source, and test evidence. It identifies findings and missing evidence; it cannot inspect changing workspace or provider state.

The main agent owns shell commands, tests, provider operations, authoritative result inspection, and acceptance against the same live tracker contract. Match the returned child identity and packet SHA-256 against the supplied snapshot, then inspect the actual result; the receipt is evidence, not acceptance or independent attestation.

The only successful completion signal is Amp's `waitForResponse` observation of the child turn's running-to-idle transition, reported as `turn-complete`. That signal proves the turn completed, not that its output is correct or accepted.

## Invocation

After reading the live contract, call the exposed `loaf_delegate` tool with `role` (`implementation` or `review`), `native_ref`, the absolute `worktree`, and the complete `packet`. Use the delegation contract for packet contents; include owned paths and acceptance criteria. For review, include line-numbered source, the exact diff, and observed test results rather than asking the child to inspect the checkout.

If preflight returns `incompatible`, report the missing capability or unsupported mode. Do not silently choose another mode, broaden tools, or route to an unrestricted child. After installing or upgrading the managed plugin, reload plugins or restart Amp so the new bytes are loaded.
