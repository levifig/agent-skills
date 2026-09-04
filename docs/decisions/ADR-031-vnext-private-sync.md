---
id: ADR-031
title: "vNext private sync uses signed E2E fact envelopes and an opaque relay"
status: Accepted
date: 2026-08-29
revised: 2026-08-29
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
- Legacy Deletion Compatibility
- Consequences
- Alternatives Considered
- Revisions

## Context

vNext continuity is one operator's closed, typed, append-only fact corpus. It must converge across trusted machines and ephemeral agent environments without turning Loaf into a tracker client, a team-memory service, or a credential broker. The shipped crypto, sync, relay, and attach packages are evidence only; vNext cannot import them or preserve their wire contract.

The relay is not trusted with plaintext, ordering, completeness, or durability. Possession of project content-key material is neither membership authority nor proof that a particular attached environment authored a fact; administrator-signed environment certificates and environment signatures provide that attribution. Relay cursors are convenient pagination state but cannot be authority.

> **Revision 2026-08-31:** Scratchpad is not part of the current vNext continuity or sync catalog. The prune certificate, bootstrap capsule, and tombstone details retained in this ADR describe strict compatibility verification for pre-cutover relay history only. Current vNext has no Scratchpad writer, projection, sync export, or prune operation.

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
11. **Terminal recovery is bounded per step, not per history.** A first-seen retired or expired producer is verified into crash-resumable local candidate rows in chunks of at most 16 frames, 16,842,752 canonical candidate bytes, and 17,632,000 referenced inbox bytes. Canonical facts, receipts, heads, and the applied cursor remain unchanged until every affected terminal fence and the complete candidate corpus validate. No lifetime source-count cap may make otherwise valid signed history unrecoverable.
12. **Physical prune carries a witnessed bootstrap capsule.** Every prune vote and the administrator-signed prune certificate bind one canonical capsule sealed once with XChaCha20-Poly1305 under a typed per-prune AEAD key derived from the stable versioned prune-bootstrap key and exact prune context. A prune ID is independent random 32-byte CSPRNG output minted before derivation and sealing; it is never derived from the manifest, capsule, nonce, ciphertext, or certificate. The second derivation binds project, channel, relay incarnation, prune ID, all four interpretation selectors, membership generation, barrier, closure-reference digest, and manifest count/digest. Sealing uses a separate random 24-byte nonce. The cleartext outer wire contains only capsule/protocol/suite/bootstrap-purpose versions, channel, relay incarnation, prune, membership generation, barrier, closure-reference digest, manifest count/digest, nonce, and ciphertext. Canonical AAD covers every cleartext outer field except ciphertext plus the credential-supplied project ID. The strictly decoded plaintext envelope repeats all four outer interpretation selectors—capsule version, protocol version, cipher suite, and bootstrap-purpose version—plus the project, channel, relay, prune, membership, barrier, closure, and manifest bindings; adds the scratchpad subject and entry count; and then lists ordered entries containing only exact prune-reference digest, one prunable fact kind, and HLC. Opening requires exact equality for every repeated outer/inner selector and binding. It contains no payload or prose. The capsule digest hashes the complete canonical outer wire, while reference and manifest digests retain their existing exact scopes. A prune persists the ID and capsule before its first vote and retries the exact bytes; any immutable mismatch conflicts. A genuinely new proposal mints a new prune ID and therefore selects a distinct derived AEAD key, so random nonce collisions across different prunes do not reuse a key/nonce pair. Fresh clients use the capsule to reconstruct authenticated tombstones and environment clocks without recovering deleted ciphertext.

The wire contract, canonical transcript, limits, and test vectors live under `vnext/sync`. vNext sync has not shipped or crossed cutover, so the capsule correction redefines its existing V1 prune-control and ephemeral-credential encodings in place: capsule fields are mandatory, pre-capsule V1 bytes fail strict decoding, and no optional-field or dual-decoder compatibility path exists. If persisted supported V1 sync bytes are discovered before cutover, implementation stops and mints a strict V2 instead. The already-pinned `golang.org/x/crypto/chacha20poly1305` import is admitted only at its exact crypto adapter file. No dependency is added or upgraded.

## Security Boundary

The protocol protects confidentiality and authenticated integrity against the network and relay. Environment signatures provide origin attribution among attached clients. It does not protect plaintext or file-backed secrets from a hostile same-UID process, malware, or a privileged user. A compromised ephemeral environment can read its explicit generations and author as itself until revocation/expiry; revocation plus rotation to a generation absent from that finite set restores future live-content confidentiality against it. A compromised trusted environment exposes the project root and can derive every future generation under that root even after relay revocation. Repair then requires a new project root, channel, keys, tokens, and explicit reattachment. No revocation retracts plaintext already copied.

The relay can deny service, reorder or withhold objects, reveal traffic metadata, or destroy storage. Returning clients detect changes to previously observed objects, arrival gaps, environment-sequence gaps, equivocation, and rollback below retained watermarks. A brand-new recovery client cannot prove that a single hostile relay disclosed its newest unseen suffix without an external witness; v1 makes no such freshness claim.

Free-form continuity prose can contain text a user copied from a tracker or a secret. The enforceable boundary is structural: sync contains no tracker model, provider adapter, or credential field. Content-level DLP is a separate decision.

The detailed threat analysis is [vNext Private Sync Threat Model](../security/vnext-private-sync-threat-model.md).

## Persistence and Convergence

Client sync metadata belongs in the same private continuity SQLite database. It entered at schema `vnext/2` and the resumable-candidate/tagged-tombstone correction advances it to `vnext/3`. This is the only way to atomically commit received facts, immutable receipts, environment heads, quarantine state, prune tombstones, and the applied cursor. The sealed outbox is derived from unreceipted local facts, so a fact cannot be stranded by a post-append enqueue failure. Credentials remain outside fact rows and sync metadata.

Terminal-history candidates also live in this private database under schema `vnext/3`. They contain already-verified canonical facts and immutable envelope metadata, use the same local-plaintext trust boundary as the canonical fact table, and never enter projections. `continuity_sync_terminal_candidates` stores staging and promoted headers; `continuity_sync_terminal_candidate_frames` stores bounded verified plaintext; and the partial unique index `ux_continuity_sync_terminal_candidates_staging_project` permits at most one header with `state = 'staging'` per project while allowing multiple promoted receipts. The rebuilt `continuity_sync_inbox` is a strict tagged union with `frame_kind IN ('sealed', 'pruned')`, bounded `frame_bytes`, and the existing `state IN ('staged', 'quarantined')` lifecycle.

Promotion deletes its plaintext frame rows and retains the small immutable promoted header permanently as the exact commit-unknown retry lookup record; LOAF-97 defines no cleanup or garbage collection for promoted headers. The receipt binds the project and candidate IDs, channel, relay incarnation, canonical pinned-authority digest including terminal fences, start and through arrivals, frame count, rolling candidate digest, post-promotion corpus digest, and resulting applied cursor. Each verification transaction admits only one bounded contiguous chunk; the accumulated candidate history has no protocol count cap. Authority drift makes a candidate non-promotable and requires an explicit discard and restart while preserving the opaque inbox.

The v2→v3 migration first verifies the exact v2 version, checksum, objects, and normalized SQL, then rebuilds the inbox and copies every legacy `sealed_envelope` byte-for-byte as `frame_kind = 'sealed'`; no legacy row is synthesized as pruned. It creates both candidate tables and the partial index empty, installs the exact v3 schema record/checksum, runs `foreign_key_check`, and commits all changes in one transaction. A crash leaves exact v2 or exact v3. A v1 database migrates through exact v2 before v3; unknown or drifted schemas fail without mutation. A v2 binary must reject v3 rather than downgrade it, and a v3 binary migrates v2 once before use.

The relay uses a separate SQLite database and schema because it stores only opaque envelopes and control-plane records. Each physical relay database has one random immutable incarnation identifier shared by every channel; clients pin it and require recovery if it changes. Arrival rows are append-only. A pruned arrival retains its fact ID, source environment and sequence, digest, and prune certificate while its ciphertext becomes `NULL`; arrival numbers are never reused.

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
- **Ephemeral project credential:** project identity, channel, expiring environment certificate and private key, expiring environment relay token, a write-generation selector, an explicit finite set of historical/current generation keys, and the typed prune-bootstrap key plus purpose version. The nonzero selector must be certificate-authorized and name exactly one carried typed key; it deterministically selects new sealing but grants no authority beyond the certificate's signed allowed-generation set. Inbound opening still selects the exact key generation named by the envelope header. The credential has no project root, admin key, owner token, or future live-content generation authority and is never persisted by Loaf. The bootstrap key intentionally remains able to open future deletion-anchor metadata obtained from the relay.

All wire encodings use fixed-field canonical JSON with unknown fields rejected, a distinct versioned prefix, and a truncated SHA-256 checksum for corruption detection. The checksum is not authentication. Because vNext sync remains unshipped and pre-cutover, adding the mandatory ephemeral write-generation selector strictly redefines V1 in place: old missing-field wires fail closed rather than receiving a default or upgrade. Recovery and attach secrets are read from a protected file, standard input, or harness secret channel, never a process-list-visible argument.

Attach is explicit and staged: validate one credential class; require HTTPS except an explicit loopback-only test mode; match the intended local project and fingerprints; fetch the full relay inventory; verify every certificate, signature, envelope, nonce, gap, HLC, and canonical fact; validate the candidate corpus; atomically install sync state; rebuild the deterministic projection; then mark the environment active. An empty relay requires an explicit create-empty-channel choice.

Environment identities are mint-once. Trusted environments remain active until explicit retirement. Ephemeral environments have independently enforceable relay-token and certificate expiries and must final-sync before terminal retirement. Those expiries gate honest relay and client admission; a signature alone cannot prove that an envelope was authored before expiry. Before expiry, the project administrator therefore signs and retains a terminal fence binding the relay database incarnation, environment certificate, final environment sequence, and final envelope digest. After expiry, a client accepts first-seen history only through that verified fence; an expired producer without one is quarantined as recovery-required. Locally retained authenticated envelopes remain readable. Membership changes increment a generation and invalidate unfinished prune barriers. A rolled-back or retired environment must reattach under a fresh identity. After ephemeral compromise, revoke it and rotate to a live-content generation outside its finite key set. After trusted/root compromise, generation rotation under the old root is insufficient: replace the project root and channel, all derived keys and relay tokens, and explicitly reattach trusted environments.

The project root also derives a computationally independent prune-bootstrap key through a dedicated HKDF domain. Recovery and trusted credentials derive it on demand; ephemeral credentials carry the explicit typed key and purpose version. The version identifies the derivation/transcript suite, not an ordinary expiry or content-generation rotation; replacing the project root and channel replaces it. Typed crypto APIs, a second per-prune derivation, and purpose-bound encodings prevent content-generation and bootstrap keys from being accepted interchangeably. This avoids accumulating an unbounded historical generation-key ring merely to recover deletion anchors. The key can reveal only the capsule's deleted-history metadata to a credential holder, not deleted payloads or future content generations.

## Legacy Deletion Compatibility

This section records the pre-cutover deletion protocol that existing relay history may still contain. It is not a current vNext feature. Exact legacy bytes may be decoded and verified so retained history fails safely; no current path creates new entries, exports old local Scratchpad rows, or physically prunes continuity facts.

v1 physically prunes only participant, message, claim, and claim-release facts from a closed scratchpad. It retains the least opening fact and every close fact so the existing deterministic fold remains valid.

A prune certificate binds one membership generation, fixed relay barrier, closed scratchpad, exact sorted manifest, manifest digest, active environment set, producer frontiers, and every active environment acknowledgement. All active environments must have applied through the barrier, have no gap/skew/conflict, have an empty outbox after observing close, and agree on the manifest. Joining environments block completion; retired identities are fenced.

The certificate also carries the encrypted prune-bootstrap capsule and its digest. Before voting, every active environment opens the capsule and proves that its exact ordered entries match the live target facts. Every vote binds the same capsule digest, and the relay verifies that equality plus the complete witness set before deleting ciphertext. Post-deletion authenticity therefore comes from the project administrator and every environment active at the barrier; the deleted environment signature and AEAD ciphertext can no longer be rechecked by a fresh client.

Each active client transactionally deletes the manifest facts and records durable tombstones before acknowledging. Only after all acknowledgements does the relay null the ciphertext while retaining opaque tombstone metadata, the certificate, and its opaque capsule bytes. A fresh client stages a pruned arrival as a distinct authenticated tombstone object, never as a sealed fact, and uses the opened capsule only to restore environment HLC/frontier invariants; no deleted fact enters the deterministic projection. A pruned arrival without the exact verified capsule and certificate is `recovery_required`. Ciphertext deleted before the capsule protocol cannot be reconstructed or silently upgraded; recovery requires an intact trusted replica or backup. Exact stale replays from active identities remain duplicates; conflicting replays fail; expired and retired identities are rejected uniformly and cannot use collision-specific responses as an oracle. Complete removal of scratchpad roots and closes requires a future compacted terminal fact and is not part of v1.

## Consequences

### Positive

- A stolen ephemeral bundle cannot derive future live-content generations, mint environments, recover the project, or reach another project.
- Relay compromise exposes traffic metadata and ciphertext, not continuity semantics or provider credentials.
- Signatures make environment sequence, gaps, conflicts, and retirement attributable instead of trusting a shared-key assertion.
- Same-database receive transactions prevent a cursor from outrunning authoritative local facts.
- Sealed-once outboxes make crash retries byte-stable and close the legacy post-commit enqueue window.
- Tombstones and membership-bound barriers make scratchpad deletion non-resurrectable without changing continuity projections.
- Bounded terminal-candidate chunks remain crash-resumable without imposing a lifetime history cap.
- Witnessed encrypted prune anchors preserve relay-only bootstrap after physical deletion without retaining deleted payload ciphertext.

### Negative

- The protocol has two cryptographic layers and a signed membership control plane rather than AEAD alone.
- Trusted credential files remain sensitive under the same-UID trust model until portable OS secret-store integration earns its own design.
- Offline trusted environments block physical prune until they return or the operator explicitly retires them with acknowledged loss risk.
- A full inventory reconciliation costs bandwidth and storage proportional to the retained opaque corpus.
- Verified terminal candidates temporarily duplicate plaintext in the same private local database until promotion or explicit discard.
- Deleting candidate rows does not promise secure erasure from SQLite WAL, free pages, snapshots, or backups; those remain within the documented local-plaintext trust boundary.
- Final terminal validation and atomic promotion still cost CPU, memory, SQLite WAL space, disk, and writer-lock time proportional to the retained valid corpus even though staging is bounded per transaction. Resource failure rolls back without imposing a protocol history cap.
- The stable prune-bootstrap key lets a stolen ephemeral bundle decrypt future deletion-anchor metadata from a cooperating hostile relay, though not deleted payloads or future content generations.

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

### Cap terminal history and promote it in one batch

A fixed lifetime count is simple and bounds one allocation, but eventually turns otherwise valid signed history into unrecoverable state. Removing only the count still loses verified work on crash and creates an unbounded request surface. Bounded disk-backed candidate chunks plus one atomic final promotion preserve recovery without making history length a protocol validity rule.

## Revisions

- 2026-08-31 - Deferred Scratchpad from current vNext; retained the deletion wire only as strict pre-cutover compatibility evidence.
- 2026-08-29 — Initial record.
- 2026-08-29 — Replaced a rejected lifetime terminal-history cap with bounded, crash-resumable verified candidate staging and atomic promotion.
- 2026-08-29 — Added the all-active-witness encrypted prune-bootstrap capsule and domain-separated bootstrap key required for fresh recovery after physical deletion.
