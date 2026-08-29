---
id: ADR-031
title: "vNext private sync uses signed E2E fact envelopes and an opaque relay"
status: Accepted
date: 2026-08-29
supersedes: null
superseded_by: null
related:
  - ADR-019
  - ADR-029
  - ADR-030
---

# ADR-031: vNext private sync uses signed E2E fact envelopes and an opaque relay

## Contents

- Context
- Decision
- Security Boundary
- Persistence and Convergence
- Attach, Recovery, and Revocation
- Scratchpad Safe Points
- Consequences
- Alternatives Considered
- Revisions

## Context

vNext continuity is one operator's closed, typed, append-only fact corpus. It must converge across trusted machines and ephemeral agent environments without turning Loaf into a tracker client, a team-memory service, or a credential broker. The shipped crypto, sync, relay, and attach packages are evidence only; vNext cannot import them or preserve their wire contract.

The relay is not trusted with plaintext, ordering, completeness, or durability. A project encryption key authenticates project membership but cannot prove which attached environment authored a fact. Relay cursors are convenient pagination state but cannot be authority. Scratchpad facts are physically removable only when an offline or rolled-back environment cannot resurrect them.

The construction is source-backed by the [Go XChaCha20-Poly1305 implementation](https://pkg.go.dev/golang.org/x/crypto/chacha20poly1305), [RFC 8439](https://www.rfc-editor.org/info/rfc8439/), [Go HKDF](https://pkg.go.dev/crypto/hkdf), [RFC 5869](https://www.rfc-editor.org/info/rfc5869/), and the standard-library [Ed25519 implementation](https://pkg.go.dev/crypto/ed25519). Bearer relay credentials require TLS, consistent with [RFC 6750](https://www.rfc-editor.org/info/rfc6750/).

## Decision

vNext private sync is a new protocol with the following fixed boundaries.

1. **Only vNext continuity facts synchronize.** The synchronized semantic allowlist is exactly `continuity.FactCatalog()`. Derived context, tracker work, provider configuration, tracker credentials, issue state, assignments, hierarchy, and workflow state have no sync representation.
2. **Each project has independent authority.** Setup generates a random 32-byte project root, random opaque relay channel ID, random project-owner relay token, and project admin Ed25519 key. There is no account-wide master secret.
3. **Content keys are generation-specific.** HKDF-SHA-256 derives one 32-byte AEAD key from the project root, project ID, protocol suite, and unsigned 32-bit generation. The generation selects exactly one key; decryption never trials an ambiguous ring.
4. **Facts use XChaCha20-Poly1305 with random 24-byte nonces.** Associated data binds protocol version, cipher suite, channel ID, fact ID, environment ID, environment sequence, key generation, previous sealed-envelope digest, environment certificate ID, and nonce. Decrypted copies of duplicated fact fields must match the authenticated header exactly.
5. **Every environment signs its own facts.** The project admin signs an environment certificate binding one random environment ID, its Ed25519 public key, trusted or ephemeral mode, membership generation, allowed key generations, and optional expiry. The environment signs an unambiguous length-prefixed transcript of the complete header and ciphertext. The relay and clients reject facts attributed to another environment.
6. **Envelopes are sealed once.** A client persists the exact sealed bytes and digest before first upload and reuses them for every retry. Same fact ID and same full-envelope digest is idempotent; the same fact ID or environment sequence with different immutable bytes is a hard conflict.
7. **Protocol metadata is not semantic authority.** The relay may see opaque channel, fact, environment, certificate, generation, sequence, arrival, digest, nonce, size, and timing metadata. Subject, fact kind, payload, HLC, observations, prose, and references remain encrypted.
8. **Inbound facts enter through a distinct atomic receive path.** Remote import preserves the original fact ID, environment sequence, HLC, versions, and canonical payload. It validates the complete candidate union so concurrent siblings can converge without weakening local authoring admission. Facts, receive receipts, environment heads, and the applied cursor commit together.
9. **Relay cursors are pagination only.** Clients persist downloaded and canonically applied cursors separately, require contiguous arrival frames, retain per-environment frontiers, and periodically reconcile the full immutable inventory. Future-skewed facts are quarantined without advancing the applied cursor or project clock; old offline facts are not rejected merely for age.
10. **Attached operation fails loudly.** Local-only operation is valid before first attach. After attach, missing credentials, an expired environment, a gap, skew quarantine, rollback, unsupported version, wrong key, or relay conflict produces explicit incomplete or recovery-required state; it never silently becomes memoryless.

The wire contract, canonical transcript, limits, and test vectors live under `vnext/sync`. The already-pinned `golang.org/x/crypto/chacha20poly1305` import is admitted only at its exact crypto adapter file. No dependency is added or upgraded.

## Security Boundary

The protocol protects confidentiality and authenticated integrity against the network and relay. Environment signatures provide origin attribution among attached clients. It does not protect plaintext or file-backed secrets from a hostile same-UID process, malware, or a privileged user. A compromised attached environment can read every generation key it received and author valid facts as itself. Revocation prevents future relay access but cannot retract copied plaintext.

The relay can deny service, reorder or withhold objects, reveal traffic metadata, or destroy storage. Returning clients detect changes to previously observed objects, arrival gaps, source-sequence gaps, equivocation, and rollback below retained watermarks. A brand-new recovery client cannot prove that a single hostile relay disclosed its newest unseen suffix without an external witness; v1 makes no such freshness claim.

Free-form continuity prose can contain text a user copied from a tracker or a secret. The enforceable boundary is structural: sync contains no tracker model, provider adapter, or credential field. Content-level DLP is a separate decision.

The detailed threat analysis is [vNext Private Sync Threat Model](../security/vnext-private-sync-threat-model.md).

## Persistence and Convergence

Client sync metadata belongs in the same private continuity SQLite database under schema line `vnext/2`. This is the only way to atomically commit received facts, immutable receipts, environment heads, quarantine state, prune tombstones, and the applied cursor. The sealed outbox is derived from unreceipted local facts, so a fact cannot be stranded by a post-append enqueue failure. Credentials remain outside fact rows and sync metadata.

The relay uses a separate SQLite database and schema because it stores only opaque envelopes and control-plane records. Arrival rows are append-only. A pruned arrival retains its fact ID, source environment and sequence, digest, and prune certificate while its ciphertext becomes `NULL`; arrival numbers are never reused.

One bounded sync pass is:

1. authenticate and compare relay, membership, producer, and applied watermarks;
2. pull and durably stage a contiguous page;
3. verify signatures and AEAD, strictly decode the closed fact contract, and atomically admit the verified prefix;
4. seal and push local unreceipted facts in environment-sequence order;
5. pull again to observe accepted and concurrent objects; and
6. acknowledge only when the outbox is empty and the applied cursor equals the observed relay head.

Environment sequences begin at one and remain contiguous. Per-environment HLC must increase with sequence. The future-skew rule is one-sided: reject or quarantine `remote_wall > trusted_now + max_future_skew`; accept arbitrarily old valid offline facts. Authenticated clocks are never clamped or rewritten.

## Attach, Recovery, and Revocation

Three credential classes are structurally distinct.

- **Project recovery credential:** project identity, relay URL and generation, channel ID, project root, project admin private key, owner relay token, write generation, and optional last signed checkpoint. It is an offline bearer secret and never enters logs, arguments, repository configuration, the relay, or an ordinary client bundle.
- **Trusted project credential:** project identity, channel, trusted environment certificate and private key, long-lived environment relay token, project root, write generation, minimum protocol version, and last observed relay checkpoint. It has no admin key or owner token.
- **Ephemeral project credential:** project identity, channel, expiring environment certificate and private key, expiring environment relay token, and an explicit finite set of historical/current generation keys. It has no project root, admin key, owner token, or future-generation derivation authority and is never persisted by Loaf.

All wire encodings use fixed-field canonical JSON with unknown fields rejected, a distinct versioned prefix, and a truncated SHA-256 checksum for corruption detection. The checksum is not authentication. Recovery and attach secrets are read from a protected file, standard input, or harness secret channel, never a process-list-visible argument.

Attach is explicit and staged: validate one credential class; require HTTPS except an explicit loopback-only test mode; match the intended local project and fingerprints; fetch the full relay inventory; verify every certificate, signature, envelope, nonce, gap, HLC, and canonical fact; validate the candidate corpus; atomically install sync state; rebuild the deterministic projection; then mark the environment active. An empty relay requires an explicit create-empty-channel choice.

Environment identities are mint-once. Trusted environments remain active until explicit retirement. Ephemeral environments expire and must final-sync before terminal retirement. Membership changes increment a generation and invalidate unfinished prune barriers. A rolled-back or retired environment must reattach under a fresh identity. Content-key rotation is required when future confidentiality from a compromised client matters.

## Scratchpad Safe Points

v1 physically prunes only participant, message, claim, and claim-release facts from a closed scratchpad. It retains the least opening fact and every close fact so the existing deterministic fold remains valid.

A prune certificate binds one membership generation, fixed relay barrier, closed scratchpad, exact sorted manifest, manifest digest, active environment set, producer frontiers, and every active environment acknowledgement. All active environments must have applied through the barrier, have no gap/skew/conflict, have an empty outbox after observing close, and agree on the manifest. Joining environments block completion; retired identities are fenced.

Each active client transactionally deletes the manifest facts and records durable tombstones before acknowledging. Only after all acknowledgements does the relay null the ciphertext while retaining opaque tombstone metadata and the certificate. Exact stale replays remain duplicates; conflicting replays fail; retired clients cannot write. Complete removal of scratchpad roots and closes requires a future compacted terminal fact and is not part of v1.

## Consequences

### Positive

- A stolen ephemeral bundle cannot derive future generations, mint environments, recover the project, or reach another project.
- Relay compromise exposes traffic metadata and ciphertext, not continuity semantics or provider credentials.
- Signatures make environment sequence, gaps, conflicts, and retirement attributable instead of trusting a shared-key assertion.
- Same-database receive transactions prevent a cursor from outrunning authoritative local facts.
- Sealed-once outboxes make crash retries byte-stable and close the legacy post-commit enqueue window.
- Tombstones and membership-bound barriers make scratchpad deletion non-resurrectable without changing continuity projections.

### Negative

- The protocol has two cryptographic layers and a signed membership control plane rather than AEAD alone.
- Trusted credential files remain sensitive under the same-UID trust model until portable OS secret-store integration earns its own design.
- Offline trusted environments block physical prune until they return or the operator explicitly retires them with acknowledged loss risk.
- A full inventory reconciliation costs bandwidth and storage proportional to the retained opaque corpus.

### Neutral

- The protocol is personal continuity, not a shared-team key hierarchy. Per-environment signatures identify one operator's replicas; they do not introduce assignments, team roles, or multi-user authorization.
- The existing continuity fact and projection contract does not change. Sync adds persistence around it and a remote receive path.
- The legacy relay remains operational for the shipped line until cutover but is not compatible with this protocol.

## Alternatives Considered

### Shared project AEAD without environment signatures

This is smaller, but every attached environment can forge another environment ID and sequence. Gap attribution, retirement fencing, and ephemeral compromise scope become assertions by holders of one shared key. Standard-library Ed25519 removes that ambiguity without a dependency.

### Tink keysets or age envelopes

Both are established libraries. Tink introduces protobuf/keyset and external key-encryption-key machinery; age lacks caller-controlled application AAD and is optimized for recipient-oriented file encryption rather than many immutable offline fact writers. Neither advantage justifies a new dependency here.

### Reuse the shipped sync and relay

This violates vNext isolation and retains ID-only duplicate handling, cursor coupling, incomplete gap detection, delete-without-tombstone behavior, and legacy tracker-related fact vocabulary.

### Separate client sync database

This preserves the continuity schema but creates an unavoidable crash boundary between received facts and the applied cursor, and between local facts and their durable outbox identity. The stronger invariant is worth an explicit `vnext/2` migration.

### Trust relay cursors and delete pruned rows

This lets a malicious or restored relay skip uncommitted data and lets stale clients resurrect deleted scratchpad facts. Cursors remain hints; arrival tombstones remain durable.

## Revisions

- 2026-08-29 — Initial record.
