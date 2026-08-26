# Substrate E2E Threat Model (LOAF-66)

Decision Date: 2026-08-26
Status: Accepted

## Scope of this document

LOAF-66 ships the **local crypto contract**: HKDF project-key derivation with a
generation read ring, AEAD wire envelopes, Emergency Kit recovery, and typed
admin vs project-client credentials. It does **not** implement the sync relay,
attach ceremony, or authorization enforcement.

## Server-visible metadata (v1 design target)

When a sync relay exists (LOAF-65), it is designed to see opaque ciphertext
blobs plus transport metadata only:

- Blob count and byte sizes per project channel
- Arrival ordering and timing
- Wire envelope cleartext: protocol version and fact id (UUIDv7 ordering leak acknowledged)
- Auth token scope (project channel binding)

The relay must store **no** cryptographic key material, no decrypted payloads,
and no semantic fact fields (kind, env_id, seq, hlc, payload).

## Server cannot see

- Master keys, project keys, recovery kit contents, or account admin secrets
- Fact kind, payload, env_id, monotonic seq, HLC, or envelope version (all AEAD-protected)
- Journal text, entity bodies, or any substrate semantics

## Write / delete capability (H1)

Locked product rule: project client credentials are **write-capable**; delete
is **operator/admin-only** (account admin secret).

Enforcement of those capabilities lives outside this crypto slice:

- **Write gating / sync upload** — LOAF-65 relay + client engine (token scope)
- **Attach / provisioning of credentials** — LOAF-67
- **Delete (tombstone + server-side blob deletion by fact id)** — LOAF-65/67 operator path

LOAF-66 only encodes the credential types and cryptographic material so those
later layers can enforce the rule without inventing a second key model.

## Audit path

1. **Crypto contract (today, LOAF-66)** — open-source `internal/crypto/` plus
   published vectors in `internal/crypto/testdata/published_vectors.json`,
   verified by `go test ./internal/crypto/...`. Auditors can check AEAD/HKDF
   behavior and that client credentials are a distinct type from admin
   credentials (no master key / account admin secret fields on the client wire).
2. **Relay inspection (when LOAF-65 lands)** — audit that relay persistence
   contains blobs + auth tokens only (empty cryptographic material set).
3. **Gap detection (LOAF-65 client engine)** — per-environment monotonic seq
   inside ciphertext enables clients to detect interior tamper/drop.

## Honest limits (v1)

- Withheld suffix vs idleness is indistinguishable without cross-environment corroboration (future work).
- Padding/traffic-analysis defenses are out of scope for v1.
- Delete semantics (tombstone + opaque blob delete by fact id) are specified here for threat-model clarity; runtime enforcement is LOAF-65/67.

## Attach ceremony

Connection provisioning, endpoint distribution, and fail-loud attach gates
(capability / identity / token-scope / gross HLC skew) are LOAF-67 scope.
