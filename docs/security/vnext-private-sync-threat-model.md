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
- Crash-resumable verified terminal candidates; immutable promoted receipts binding the exact authority, candidate range/digest, post-promotion corpus digest, and resulting watermark; prune-bootstrap capsule ciphertext and digests; and the typed domain-separated prune-bootstrap key.
- The negative authority boundary: no tracker-owned model, provider credential, account-wide root, or another project's key enters sync.

## Trust Boundaries

1. **Caller to continuity API.** User-controlled identifiers and prose enter only named typed methods and bounded validators. No generic append surface exists.
2. **Local process to SQLite.** Filesystem controls protect against other users and accidental path substitution, not hostile same-UID or privileged processes.
3. **Continuity adapter to sync.** Exact persisted fact fields cross through an internal-only wire. Sync receives no raw database handle and cannot invent fact kinds.
4. **Credential source to client.** Recovery, trusted, and ephemeral classes carry materially different authority and reject unknown fields.
5. **Client to relay.** TLS protects bearer tokens. The relay verifies project/environment scope, certificate, signature, limits, sequence, and immutable digest but never decrypts content.
6. **Relay to client.** Every frame is hostile until bounded, signature-verified, AEAD-opened, outer/inner matched, strictly decoded, gap/skew checked, and admitted as one valid candidate union.
7. **Terminal candidate staging to canonical continuity.** Chunks of at most 16 frames, 16,842,752 canonical candidate bytes, and 17,632,000 referenced inbox bytes may enter `continuity_sync_terminal_candidate_frames` only after exact authority, certificate, envelope, fact, chain, nonce, HLC, and inbox checks. `continuity_sync_terminal_candidates` and its partial unique staging-project index permit one staging candidate per project and permanent promoted receipts. The strict inbox `frame_kind` tag separates sealed from pruned bytes. No candidate affects a projection or applied cursor until the complete terminal corpus and every fence validate in one canonical promotion.
8. **Continuity schema v2 to v3.** Migration accepts only the exact known version, checksum, normalized SQL, and object inventory; copies each legacy inbox byte exactly as `frame_kind = 'sealed'`; creates no synthetic pruned row; installs both candidate tables and the partial staging index empty; validates foreign keys; and commits version/checksum/objects/data together. Unknown, drifted, newer, or downgrade attempts fail without mutation.
9. **Scratchpad retention to deletion.** Physical deletion requires a membership-bound signed certificate, a capsule digest agreed by every active witness, and durable local/relay tombstones. A fresh client treats the capsule as authenticated deletion authority, not as a fact or projection snapshot.

## Attacker Capabilities

- An unauthenticated network attacker can send malformed, oversized, replayed, or concurrent requests and attempt resource exhaustion.
- A malicious or compromised relay controls availability, ordering, returned prefixes, and storage while observing opaque metadata, sizes, timing, and IP information.
- A stolen ephemeral bundle exposes one project's explicitly included live-content generations, lets the attacker author as that certified environment until expiry/revocation, and retains the ability to decrypt future deletion-anchor metadata obtained from a cooperating relay through its stable prune-bootstrap key.
- A compromised trusted environment exposes that project's root and historical plaintext, can derive every future content generation under that root even after relay revocation, and can author valid facts as itself while admitted. It lacks project admin signing authority and owner relay authority.
- A compromised recovery credential exposes the project root, administrator signing key, and owner relay token. The attacker can mint authority and perform owner operations for that project; recovery cannot make this credential safe again, so containment requires a new root, channel, keys, tokens, and explicit reattachment.
- A stolen administrator key can sign environment certificates and control, fence, and prune objects but cannot decrypt content or authenticate owner operations to an honest relay without the corresponding bearer token.
- A stolen owner relay token can invoke owner endpoints but cannot create a valid authority-changing object without the administrator signature required by those endpoints.
- Physical deletion still requires matching acknowledgements from every environment active at the fixed barrier; an honest active witness rejects a false capsule or manifest. Compromise of the administrator plus every applicable active-witness signing key defeats that quorum assumption.
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
| P0 | Stolen ephemeral bundle grants permanent live-content/authorship or cross-project access | One project only; explicit finite live-content generation keys; independently enforced token/certificate expiry; admin-signed terminal producer fence; no root/admin/owner secret | Copied plaintext and received keys cannot be revoked retroactively; an unfenced expired tail is recovery-required; the stable bootstrap key retains future deletion-anchor metadata access from a cooperating relay |
| P0 | Recovery credential, administrator key, or owner token is substituted across credential classes | Closed credential schemas; typed keys/tokens; exact project/channel fingerprints; admin signature plus owner-token checks for owner mutations; active-witness quorum for prune | Recovery-credential compromise is full project takeover and requires root/channel replacement |
| P0 | Trusted project-root compromise survives relay revocation or ordinary generation rotation | Replace project root and channel, all derived keys and tokens, and explicitly reattach; never treat an old-root generation increment as repair | Copied plaintext and old-root ciphertext remain compromised |
| P0 | Malicious checkout redirects attach or supplies secrets | Repository carries only expected opaque identity/fingerprint; endpoint and secret arrive out of band; explicit display/confirmation | Operator can still approve a malicious endpoint |
| P1 | Relay omits a prefix, interior object, or previously observed tail | Environment sequence begins at one; previous-envelope digest; contiguous arrival pages; retained frontiers/inventory digests/checkpoints | A fresh recovery client cannot prove an unseen newest suffix |
| P1 | Future HLC dominates projections or old offline work is rejected | Quarantine future-only skew before insert; accept old valid clocks; enforce increasing HLC per environment sequence | A quarantined valid fact blocks complete convergence until resolved |
| P1 | Crash loses outbox work or advances a cursor past facts | Derive pending from fact receipts; persist sealed bytes before upload; receive facts/heads/applied cursor in one transaction | Disk loss still needs relay/recovery bootstrap |
| P1 | Retired or rolled-back environment reuses sequence | Relay producer watermark, signed identity, mint-once retirement fence, rollback handshake | Force retirement may acknowledge unpublished loss |
| P1 | Valid token exhausts relay/client resources | Exact object/page/body limits, quotas, timeouts, bounded concurrency, reject before signature/decrypt where possible | Authorized clients can still consume their quota |
| P1 | Scratchpad content resurrects after prune | Fixed barrier, membership-generation manifest, all-active acknowledgements, signed prune certificate, permanent tombstones | Offline trusted environments delay pruning until retired |
| P1 | Relay substitutes, omits, reorders, or forges deleted-history anchors | Canonical XChaCha20-Poly1305 bootstrap capsule; exact AAD/plaintext manifest/reference binding; capsule digest in every active vote and administrator-signed certificate; strict tagged tombstone replay | Post-deletion authenticity depends on the administrator plus the complete active witness set because deleted ciphertext cannot be reverified |
| P1 | A same-prune retry changes capsule bytes or repeats a key/nonce pair | Typed per-prune AEAD key derived from the stable bootstrap key and exact prune identity; random 24-byte nonce; sealed-once capsule persisted before voting; exact-byte retry; immutable prune/capsule digest binding | Random generation still depends on a sound OS CSPRNG; retry safety depends on durable pre-vote persistence |
| P1 | Oversized terminal history exhausts memory or becomes unrecoverable | Exact 16-frame and byte-bounded verified-candidate chunks persisted locally; no lifetime history-count cap; one staging candidate per project; atomic canonical promotion only after every fence and full candidate fold | Final validation remains corpus-proportional and can exhaust finite memory, disk, WAL, or writer-lock budgets; failure must leave canonical state unchanged |
| P1 | Schema drift, crash, or an older binary mis-tags legacy sealed bytes as pruned during v2→v3 | Exact v2 checksum/object/normalized-SQL preflight; one transaction copies legacy bytes only as sealed, creates empty candidate tables/index, validates FKs, and installs v3 version/checksum; v2 rejects v3 | Finite disk or lock failures can defer migration but must leave exact v2 retryable |
| P2 | Free-form prose contains copied tracker secrets | Structural exclusion; document that content DLP is not provided | E2E replicates user-entered secret prose |
| P2 | Relay correlates usage patterns | Document visible metadata; no anonymity claim | Padding and traffic-shaping remain future work |

## Credential and Recovery Boundary

Project setup generates an independent random project root, channel ID, relay owner token, and Ed25519 admin key. The project root derives purpose-separated key families through HKDF-SHA-256: live-content AEAD keys use project-specific salt plus suite/generation info, while the bootstrap family uses its own domain. Relay tokens are independently random and are never derived from the project root.

The project root separately derives a computationally independent prune-bootstrap key through a dedicated HKDF domain. Recovery and trusted clients derive it; ephemeral credentials receive the explicit typed key because their bounded generation-key lists must not grow with every historical prune. Its purpose version identifies the derivation/transcript suite and does not expire or rotate with live-content generations. A typed crypto API derives a second AEAD key from the bootstrap key and exact prune identity, then seals the capsule with XChaCha20-Poly1305 and a random 24-byte nonce. The outer wire exposes only capsule/protocol/suite/bootstrap-purpose versions, channel, relay incarnation, prune, membership generation, barrier, closure-reference digest, manifest count/digest, nonce, and ciphertext. Canonical AAD covers those cleartext fields except ciphertext plus the credential-supplied project ID. The plaintext envelope repeats all four outer interpretation selectors—capsule version, protocol version, cipher suite, and bootstrap-purpose version—plus the project/channel/relay/prune/membership/barrier/closure/manifest bindings, adds scratchpad subject and entry count, and contains ordered entries of exact prune-reference digest, prunable fact kind, and HLC. Opening requires exact equality for every repeated selector and binding. The capsule digest covers the complete canonical outer wire; acknowledgements and the administrator certificate bind that digest. Purpose-bound derivation and typed APIs reject content-generation/bootstrap-key substitution. Capsule bytes are persisted before the first vote and reused exactly. It never contains deleted payload or prose.

The recovery credential contains full project recovery/admin authority and is an offline bearer secret. A trusted credential contains the project root and its own environment authority but no admin key or owner token. An ephemeral credential contains explicit generation keys, the typed stable prune-bootstrap key, and its own expiring environment authority. None contains tracker or provider credentials.

Recovery encoding is versioned fixed-field canonical JSON plus a truncated SHA-256 corruption checksum. The checksum is not a password hash, MAC, or proof of authenticity. Recovery must fetch and validate a full inventory, mint a fresh environment identity, and reach an attached projection before success. Root compromise requires a new root, channel, tokens, and explicit reattachment; ordinary recovery cannot make a compromised root safe.

Secrets must enter through a protected file, standard input, or harness secret channel. They never enter command arguments, logs, repository configuration, continuity facts, sync diagnostics, or relay plaintext. A portable OS keychain is future hardening; a `0600` file retains the documented same-UID limitation.

## Relay Metadata and Authorization

The relay stores only:

- opaque channel, environment, fact, certificate, and prune identifiers;
- transport/suite/key generation, source and arrival sequences, nonce, digest, ciphertext size, timing, and ciphertext or prune tombstone;
- admin public key, signed environment certificates/control objects including opaque capsule bytes and their digest inside each prune certificate, high-entropy token hashes, expiry, membership generation, producer/applied watermarks, quotas, and retirement state.

It stores no project root, generation key, signing private key, bearer token plaintext, decrypted fact kind/subject/payload/HLC, external locator, tracker schema, provider credential, assignment, workflow, or hierarchy.

Bearer tokens are project- and environment-scoped high-entropy values looked up by a public token ID and compared in constant time. Owner control operations also require an admin-signed object. HTTPS is mandatory except an explicit loopback-only test mode. Authentication errors do not disclose whether a token, channel, or environment exists.

Ephemeral certificate and relay-token expiry are honest admission policy, not cryptographic proof of authoring time. A hostile relay can accept a newly signed envelope after expiry and lie about timing, and encrypted HLC cannot establish when the signature was made. Before expiry, an administrator-signed terminal fence must be retained outside the relay and bind the relay database incarnation, certificate, final environment sequence, and final digest. First-seen expired history is accepted only at or below that verified fence; an absent fence or later environment sequence is quarantined. Historical opening without a clock is limited to already retained authenticated bytes or bytes covered by that fence.

## Failure Behavior

- Unknown protocol, suite, key generation, certificate, fact kind, field, or noncanonical payload fails closed and does not advance applied progress.
- Wrong keys, invalid signatures, binding mismatch, nonce reuse, immutable conflicts, equivocation, gaps, and rollback are machine-stable secret-free errors.
- Future-skewed frames remain durably quarantined but unapplied; later frames may stage but cannot move the applied prefix past them.
- Verified terminal chunks survive process restart but remain noncanonical; authority drift, an incomplete fence, future skew, invalid candidate fold, or a conflicting retry leaves facts, receipts, heads, and applied progress unchanged. Authority drift requires an explicit discard before restaging from the preserved opaque inbox. Promotion atomically deletes plaintext child rows, marks the exact receipt promoted with its authority/range/candidate/corpus/watermark bindings, and advances the applied cursor; an exact lost-response retry returns that prior outcome, while any altered receipt summary conflicts.
- Old offline facts are accepted when signature, sequence, HLC monotonicity, canonical fact shape, and causal closure are valid.
- Relay cursors never prove completeness. Lower head, missing known object, changed digest, or changed relay generation requires recovery.
- Relay certificate and bearer-token expiry reject new requests uniformly before signature verification or collision-specific lookup. A terminal fence, not relay time, bounds first-seen expired history.
- Once attached, missing authority or relay failure is visible. Ephemeral writes require relay acknowledgement or a durable encrypted outbox that survives the environment.
- A pruned arrival without an exact verified capsule/certificate is `recovery_required`. Pre-capsule ciphertext that has already been deleted cannot be reconstructed or silently upgraded.

The relay cannot be forced to provide availability. A local replica and independent backup remain the durability boundary.

## Scratchpad Deletion Boundary

The first safe-point implementation retains `scratchpad.opened` and every `scratchpad.closed` fact and prunes only participant, message, claim, and claim-release facts. This preserves current causal and terminal projection semantics.

One prune run binds membership generation `G`, active set `A(G)`, fixed relay barrier `B`, a sorted manifest of exact fact/source/arrival/digest tuples, manifest digest/count, producer frontiers, and the closed scratchpad. Every active environment must apply through `B`, have no gap/skew/conflict, empty its outbox after learning close, and report the same manifest. Joining environments invalidate the run; retired identities are terminally fenced.

Clients record local tombstones transactionally with deletion. After every active client acknowledges, the relay nulls ciphertext but retains arrival/environment identity, digest, and certificate. A restored relay or stale client cannot turn a tombstoned ID or consumed environment sequence back into live content.

Before any active client votes, it opens the canonical prune-bootstrap capsule and compares each ordered entry with its live target fact. The vote binds the capsule digest; the administrator-signed certificate binds the same capsule and the complete witness set. Fresh clients verify that control chain, open the capsule, and atomically replay tagged tombstone arrivals. Capsule HLC anchors advance source frontiers but never create facts or projection records.

## Verification and Review

Implementation is not approved until all of these are true:

- Published vectors pin HKDF, AAD, canonical transcript, XChaCha envelope, Ed25519 certificate/signature, checksum, and cross-platform decoding.
- Tests cover wrong key, wrong project/channel/fact/environment/generation, altered nonce/ciphertext/signature, unsupported versions, nonce reuse, unknown fields/kinds, size limits, and redacted errors.
- Two- and three-replica delivery permutations converge byte-identically, including concurrent siblings and terminal facts.
- Lost responses, duplicate pages, process crashes, cursor boundaries, exact/conflicting duplicates, source/arrival gaps, old clocks, future skew, rollback, retirement, recovery, and ephemeral expiry fail as specified.
- Terminal recovery with at least 4,097 valid arrivals and maximum-byte frames resumes across 16-frame candidate chunks and process restarts, rejects altered retries, and then promotes all canonical state atomically. Fault tests inject `SQLITE_FULL`, cancellation, crash, and busy-lock failures during final validation/promotion and repeated authority-drift restaging; every failure leaves canonical facts, receipts, heads, inbox, candidate receipt, and applied cursor in the specified all-old or all-new state.
- Relay schema inspection proves opaque-only storage and absence of tracker/provider fields and secret plaintext.
- Attach is staged and atomic for populated, empty, wrong-key, mismatched-project, gap, skew, unsupported-version, and rollback cases.
- Safe-point tests cover laggards, joining/retired environments, pending outboxes, manifest disagreement, membership change, partial local deletion, relay tombstoning, stale replay, and fresh bootstrap after prune.
- Capsule vectors and adversarial tests cover wrong keys and every project/channel/relay/prune/barrier/manifest/reference/subject/kind/HLC binding, witness disagreement, missing legacy capsules, and more than 64 historical content generations.
- Capsule persistence tests cover exact lost-response retry, altered same-prune bytes and nonce, typed content/bootstrap-key substitution, random-source failure, secret-free open failures, and crash before and after the first witness vote.
- Terminal-receipt tests lose the response immediately after promotion, require exact retry to return the prior success, mutate every bound summary field, exercise authority drift between chunks and immediately before promotion, prove discard preserves opaque inbox bytes, and prove promotion/discard remove every plaintext child row while only promotion retains the immutable receipt.
- Migration tests cover exact v1→v2→v3 and direct v2→v3, populated staged/quarantined rows copied byte-identically as sealed, empty candidate tables, partial-index/FK enforcement, unknown or drifted schema rejection without mutation, v2-binary rejection of v3, reopen idempotency, and fault injection proving a crash leaves exact v2 or exact v3 with no synthesized pruned tag.
- Legacy-deletion tests keep a pre-capsule pruned arrival at `recovery_required` without advancing cursor, environment head, facts, tombstones, or projection; only an explicitly selected intact trusted replica or backup retaining the authenticated pre-deletion material may repair and resume it.
- A fresh independent reviewer traces effective credential paths, protocol bytes, database transactions, and relay authorization and approves or records findings. Architecture review alone is insufficient.

The cryptographic primitives and transport requirements are grounded in [Go XChaCha20-Poly1305](https://pkg.go.dev/golang.org/x/crypto/chacha20poly1305), [RFC 8439](https://www.rfc-editor.org/info/rfc8439/), [Go cipher.AEAD](https://pkg.go.dev/crypto/cipher#AEAD), [Go HKDF](https://pkg.go.dev/crypto/hkdf), [RFC 5869](https://www.rfc-editor.org/info/rfc5869/), [Go Ed25519](https://pkg.go.dev/crypto/ed25519), and [RFC 6750](https://www.rfc-editor.org/info/rfc6750/).

## Residual Risks

- A hostile relay can deny service, selectively hide a newest unseen suffix, reveal metadata, or destroy storage.
- A compromised ephemeral environment retains copied plaintext and its explicit content keys; revocation plus an undisclosed generation stops future live-content access but not future deletion-anchor metadata obtained with its stable bootstrap key.
- A compromised trusted environment retains the project root and can derive future old-root generations after relay revocation; only root/channel replacement restores future confidentiality.
- Recovery root material alone cannot prove freshness to a brand-new device without a trusted checkpoint outside the relay.
- File-backed local plaintext and secrets remain exposed to same-UID malware, privileged processes, and unencrypted backups.
- Revocation cannot retract plaintext or keys already copied from a client.
- User-authored prose can contain secrets despite structural tracker/provider exclusion.
- Environment signatures add attribution, not multi-user trust or benevolence.
- A stolen ephemeral credential that retains the stable prune-bootstrap key can decrypt future deletion-anchor metadata obtained from a cooperating relay, but not deleted payloads or future live-content generations.
- Candidate-frame deletion removes logical rows but does not securely erase verified plaintext from SQLite WAL, free pages, filesystem snapshots, or backups.
- Valid terminal history can exceed finite memory, disk, WAL, or lock budgets during final atomic validation even though staging is bounded; such a failure is operational, not a protocol history rejection, and must preserve the retryable candidate.
- Compromise of the recovery credential is a full project-authority compromise; ordinary revocation or recovery cannot repair it without replacing the project root and channel.
