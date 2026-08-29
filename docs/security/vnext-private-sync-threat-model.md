# vNext Private Sync Threat Model

## Contents

- Status and Scope
- System and Data Flow
- Assets
- Trust Boundaries
- Attacker Capabilities
- Security Objectives
- Threat Scenarios and Controls
- Credential and Recovery Boundary
- Relay Metadata and Authorization
- Failure Behavior
- Scratchpad Deletion Boundary
- Verification and Review
- Residual Risks

## Status and Scope

Status: design baseline awaiting implementation review.

This threat model covers the vNext private continuity sync protocol selected in [ADR-031](../decisions/ADR-031-vnext-private-sync.md). It is a source-backed architecture analysis, not a claim that unimplemented or unreviewed code is secure. Criterion approval requires a fresh review of the implemented construction, vectors, storage, relay authorization, recovery, revocation, convergence, and pruning behavior.

The synchronized corpus is exactly the closed fact catalog in `vnext/continuity/facts.go`: project identity, journal entries, wraps, sparks, ideas, decisions, explorations, checkpoints, findings, handoffs, scratchpad coordination, opaque external references, and verification evidence. Derived context is a read-time projection and never synchronizes. Tracker state, provider adapters, service credentials, assignments, workflow, hierarchy, and shared-team memory are out of scope.

## System and Data Flow

```text
typed local authoring
        |
        v
plaintext continuity SQLite -- exact internal fact export --> sealed-once outbox
        ^                                                        |
        |                                                        v
atomic validated receive <--- hostile network/relay <--- signed E2E envelope
        |
        v
deterministic read-time projection
```

The local continuity database is plaintext and trusted under the filesystem authority documented in ADR-030. End-to-end encryption protects network transit and relay storage. Client sync metadata shares the continuity database so fact receipt and applied progress commit atomically. Credentials are separate from facts and sync tables. The relay has a distinct opaque database.

## Assets

- Private continuity plaintext and the exact canonical fact corpus.
- Fact identity, project isolation, environment provenance, sequence, HLC, causal links, and deterministic projection results.
- Project root, generation keys, admin signing key, environment signing keys, relay bearer tokens, recovery credential, and signed certificates/control objects.
- Sealed outbox bytes, envelope digests, receive receipts, environment frontiers, applied watermarks, signed checkpoints, prune certificates, and anti-resurrection tombstones.
- The negative authority boundary: no tracker-owned model, provider credential, account-wide root, or another project's key enters sync.

## Trust Boundaries

1. **Caller to continuity API.** User-controlled identifiers and prose enter only named typed methods and bounded validators. No generic append surface exists.
2. **Local process to SQLite.** Filesystem controls protect against other users and accidental path substitution, not hostile same-UID or privileged processes.
3. **Continuity adapter to sync.** Exact persisted fact fields cross through an internal-only wire. Sync receives no raw database handle and cannot invent fact kinds.
4. **Credential source to client.** Recovery, trusted, and ephemeral classes carry materially different authority and reject unknown fields.
5. **Client to relay.** TLS protects bearer tokens. The relay verifies project/environment scope, certificate, signature, limits, sequence, and immutable digest but never decrypts content.
6. **Relay to client.** Every frame is hostile until bounded, signature-verified, AEAD-opened, outer/inner matched, strictly decoded, gap/skew checked, and admitted as one valid candidate union.
7. **Scratchpad retention to deletion.** Physical deletion requires a membership-bound signed certificate and durable local/relay tombstones.

## Attacker Capabilities

- An unauthenticated network attacker can send malformed, oversized, replayed, or concurrent requests and attempt resource exhaustion.
- A malicious or compromised relay controls availability, ordering, returned prefixes, and storage while observing opaque metadata, sizes, timing, and IP information.
- A stolen ephemeral bundle exposes one project's explicitly included generations and lets the attacker author as that certified environment until expiry/revocation.
- A compromised trusted environment exposes that project's root and historical plaintext and can author valid facts as itself. It lacks project admin signing authority and owner relay authority.
- A malicious checkout or tracker record can influence caller-supplied prose but cannot choose attach credentials, silently redirect the relay, or expand the sync allowlist.
- A same-UID local process can ordinarily read plaintext SQLite and file-backed secrets. This actor is outside the current filesystem control.

## Security Objectives

- Confidentiality and authenticated integrity of semantic facts against network and relay attackers.
- Project isolation with no account-wide secret and no provider credential in protocol or persistence.
- Environment attribution, mint-once identity, terminal retirement, and scoped ephemeral authority.
- Exact idempotency and loud immutable conflict rather than ID-only duplicate acceptance.
- Detection of known rollback, leading/interior source gaps, relay arrival gaps, equivocation, nonce reuse, and unacceptable future skew.
- Crash-safe progress: sealed bytes precede upload; verified facts and applied cursors commit together.
- Explicit attach/recovery and hard failure after attachment rather than silent empty or memoryless operation.
- Non-resurrectable scratchpad pruning while preserving the existing projection's opening and close anchors.
- Bounded secret-free diagnostics and request/page/storage limits before expensive cryptographic work.

## Threat Scenarios and Controls

The scenarios below are design hypotheses until implementation testing or review validates them.

| Priority | Scenario | Control | Residual Risk |
|----------|----------|---------|---------------|
| P0 | Relay tampers with or relocates ciphertext | XChaCha20-Poly1305; complete routing header as AAD; environment signature; exact outer/inner equality | Relay can withhold or destroy valid ciphertext |
| P0 | Attached client impersonates another environment | Admin-signed environment certificate and Ed25519 envelope signature | Compromised client can still author as itself |
| P0 | Same fact ID is preempted with different bytes | Sealed-once durable outbox; relay digest equality; local full immutable fact comparison; hard conflict | A valid compromised writer can cause an attributable denial of service |
| P0 | Stolen ephemeral bundle grants permanent or cross-project access | One project only; explicit finite generation keys; expiring token/certificate; no root/admin/owner secret | Copied plaintext and received keys cannot be revoked retroactively |
| P0 | Malicious checkout redirects attach or supplies secrets | Repository carries only expected opaque identity/fingerprint; endpoint and secret arrive out of band; explicit display/confirmation | Operator can still approve a malicious endpoint |
| P1 | Relay omits a prefix, interior object, or previously observed tail | Source sequence begins at one; previous-envelope digest; contiguous arrival pages; retained frontiers/inventory digests/checkpoints | A fresh recovery client cannot prove an unseen newest suffix |
| P1 | Future HLC dominates projections or old offline work is rejected | Quarantine future-only skew before insert; accept old valid clocks; enforce increasing HLC per source sequence | A quarantined valid fact blocks complete convergence until resolved |
| P1 | Crash loses outbox work or advances a cursor past facts | Derive pending from fact receipts; persist sealed bytes before upload; receive facts/heads/applied cursor in one transaction | Disk loss still needs relay/recovery bootstrap |
| P1 | Retired or rolled-back environment reuses sequence | Relay producer watermark, signed identity, mint-once retirement fence, rollback handshake | Force retirement may acknowledge unpublished loss |
| P1 | Valid token exhausts relay/client resources | Exact object/page/body limits, quotas, timeouts, bounded concurrency, reject before signature/decrypt where possible | Authorized clients can still consume their quota |
| P1 | Scratchpad content resurrects after prune | Fixed barrier, membership-generation manifest, all-active acknowledgements, signed prune certificate, permanent tombstones | Offline trusted environments delay pruning until retired |
| P2 | Free-form prose contains copied tracker secrets | Structural exclusion; document that content DLP is not provided | E2E replicates user-entered secret prose |
| P2 | Relay correlates usage patterns | Document visible metadata; no anonymity claim | Padding and traffic-shaping remain future work |

## Credential and Recovery Boundary

Project setup generates independent random content root, channel ID, relay owner token, and Ed25519 admin key. The content root derives only generation AEAD keys through HKDF-SHA-256 with a project-specific salt and suite/generation info. Relay tokens are independently random and are never derived from the content root.

The recovery credential contains full project recovery/admin authority and is an offline bearer secret. A trusted credential contains the project root and its own environment authority but no admin key or owner token. An ephemeral credential contains only explicit generation keys and its own expiring environment authority. None contains tracker or provider credentials.

Recovery encoding is versioned fixed-field canonical JSON plus a truncated SHA-256 corruption checksum. The checksum is not a password hash, MAC, or proof of authenticity. Recovery must fetch and validate a full inventory, mint a fresh environment identity, and reach an attached projection before success. Root compromise requires a new root, channel, tokens, and explicit reattachment; ordinary recovery cannot make a compromised root safe.

Secrets must enter through a protected file, standard input, or harness secret channel. They never enter command arguments, logs, repository configuration, continuity facts, sync diagnostics, or relay plaintext. A portable OS keychain is future hardening; a `0600` file retains the documented same-UID limitation.

## Relay Metadata and Authorization

The relay stores only:

- opaque channel, environment, fact, certificate, and prune identifiers;
- transport/suite/key generation, source and arrival sequences, nonce, digest, ciphertext size, timing, and ciphertext or prune tombstone;
- admin public key, signed environment certificates/control objects, high-entropy token hashes, expiry, membership generation, producer/applied watermarks, quotas, and retirement state.

It stores no project root, generation key, signing private key, bearer token plaintext, decrypted fact kind/subject/payload/HLC, external locator, tracker schema, provider credential, assignment, workflow, or hierarchy.

Bearer tokens are project- and environment-scoped high-entropy values looked up by a public token ID and compared in constant time. Owner control operations also require an admin-signed object. HTTPS is mandatory except an explicit loopback-only test mode. Authentication errors do not disclose whether a token, channel, or environment exists.

## Failure Behavior

- Unknown protocol, suite, key generation, certificate, fact kind, field, or noncanonical payload fails closed and does not advance applied progress.
- Wrong keys, invalid signatures, binding mismatch, nonce reuse, immutable conflicts, equivocation, gaps, and rollback are machine-stable secret-free errors.
- Future-skewed frames remain durably quarantined but unapplied; later frames may stage but cannot move the applied prefix past them.
- Old offline facts are accepted when signature, sequence, HLC monotonicity, canonical fact shape, and causal closure are valid.
- Relay cursors never prove completeness. Lower head, missing known object, changed digest, or changed relay generation requires recovery.
- Once attached, missing authority or relay failure is visible. Ephemeral writes require relay acknowledgement or a durable encrypted outbox that survives the environment.

The relay cannot be forced to provide availability. A local replica and independent backup remain the durability boundary.

## Scratchpad Deletion Boundary

The first safe-point implementation retains `scratchpad.opened` and every `scratchpad.closed` fact and prunes only participant, message, claim, and claim-release facts. This preserves current causal and terminal projection semantics.

One prune run binds membership generation `G`, active set `A(G)`, fixed relay barrier `B`, a sorted manifest of exact fact/source/arrival/digest tuples, manifest digest/count, producer frontiers, and the closed scratchpad. Every active environment must apply through `B`, have no gap/skew/conflict, empty its outbox after learning close, and report the same manifest. Joining environments invalidate the run; retired identities are terminally fenced.

Clients record local tombstones transactionally with deletion. After every active client acknowledges, the relay nulls ciphertext but retains arrival/source identity, digest, and certificate. A restored relay or stale client cannot turn a tombstoned ID or consumed source sequence back into live content.

## Verification and Review

Implementation is not approved until all of these are true:

- Published vectors pin HKDF, AAD, canonical transcript, XChaCha envelope, Ed25519 certificate/signature, checksum, and cross-platform decoding.
- Tests cover wrong key, wrong project/channel/fact/environment/generation, altered nonce/ciphertext/signature, unsupported versions, nonce reuse, unknown fields/kinds, size limits, and redacted errors.
- Two- and three-replica delivery permutations converge byte-identically, including concurrent siblings and terminal facts.
- Lost responses, duplicate pages, process crashes, cursor boundaries, exact/conflicting duplicates, source/arrival gaps, old clocks, future skew, rollback, retirement, recovery, and ephemeral expiry fail as specified.
- Relay schema inspection proves opaque-only storage and absence of tracker/provider fields and secret plaintext.
- Attach is staged and atomic for populated, empty, wrong-key, mismatched-project, gap, skew, unsupported-version, and rollback cases.
- Safe-point tests cover laggards, joining/retired environments, pending outboxes, manifest disagreement, membership change, partial local deletion, relay tombstoning, stale replay, and fresh bootstrap after prune.
- A fresh independent reviewer traces effective credential paths, protocol bytes, database transactions, and relay authorization and approves or records findings. Architecture review alone is insufficient.

The cryptographic primitives and transport requirements are grounded in [Go XChaCha20-Poly1305](https://pkg.go.dev/golang.org/x/crypto/chacha20poly1305), [RFC 8439](https://www.rfc-editor.org/info/rfc8439/), [Go cipher.AEAD](https://pkg.go.dev/crypto/cipher#AEAD), [Go HKDF](https://pkg.go.dev/crypto/hkdf), [RFC 5869](https://www.rfc-editor.org/info/rfc5869/), [Go Ed25519](https://pkg.go.dev/crypto/ed25519), and [RFC 6750](https://www.rfc-editor.org/info/rfc6750/).

## Residual Risks

- A hostile relay can deny service, selectively hide a newest unseen suffix, reveal metadata, or destroy storage.
- A compromised environment can read delivered history and author valid facts as itself until revocation.
- Recovery root material alone cannot prove freshness to a brand-new device without a trusted checkpoint outside the relay.
- File-backed local plaintext and secrets remain exposed to same-UID malware, privileged processes, and unencrypted backups.
- Revocation cannot retract plaintext or keys already copied from a client.
- User-authored prose can contain secrets despite structural tracker/provider exclusion.
- Environment signatures add attribution, not multi-user trust or benevolence.
