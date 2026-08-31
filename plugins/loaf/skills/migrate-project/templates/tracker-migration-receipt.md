# Tracker Migration Receipt Template

Return this receipt to the operator after the final verification read. Do not persist it unless requested.

```markdown
# Tracker Migration Receipt

## Destination

- Provider module: [provider slug and contract version]
- Provider: [verified native provider name]
- Workspace/organization: [verified name and native ID]
- Team/repository: [verified name and native ID]
- Project-like resources: [none, or one entry per verified project/milestone/epic with kind, name, native ID, and URL]
- Source project: [packet project label hint and ID]
- Packet contract: tracker-migration/v1

## Confirmed Mapping Policy

- Statuses: [source → destination mapping]
- Kinds: [source → destination type/label mapping]
- Hierarchy: [native parent/child or confirmed fallback]
- Relationships: [native links or confirmed fallback]
- Batch size: [1–10]

## Root Destination Plan

| Source root alias/ID | Destination kind | Confirmed display name/casing | Collision class | Hierarchy strategy |
|----------------------|------------------|-------------------------------|-----------------|--------------------|
| [`DOJO-4`] | [project/milestone/epic/issue] | [provider display name] | [create/adopt/no-op/conflict] | [project membership or issue hierarchy] |

## Verified Mappings

| Source alias/ID | Source role | Destination kind | Class | Destination ID | Destination URL | Container/parent verified | Status verified | Notes |
|-----------------|-------------|------------------|-------|----------------|-----------------|---------------------------|-----------------|-------|
| [`DOJO-4`] | root | project | create | [native project ID] | [URL] | [destination verified] | [yes/n/a] | [kind, marker, and re-read evidence] |
| [`DOJO-5`] | child | issue | create | [native issue ID] | [URL] | [project membership verified] | [yes] | [identity and re-read evidence] |

## Conflicts and Failures

| Source alias/ID | State | Evidence | Required operator decision or safe retry |
|-----------------|-------|----------|------------------------------------------|
| [source] | [conflict/failed/unverified] | [what was re-read] | [next action] |

## Verification Summary

- Packet issues: [count]
- Verified destination mappings: [count]
- Verified project/milestone/epic mappings: [count]
- Verified issue mappings: [count]
- Conflicts: [count]
- Failed or unverified: [count]
- Hierarchy/relationships verified: [count/count]
- Mappings inferred from provider order: none
- Local issue mutations: none
- Local continuity writes: invocation journal entry only
- Private continuity migrated: none

## Boundary

The local Loaf issue rows remain untouched and available as migration history; only the required workflow invocation was added to the private journal. The destination tracker is canonical for migrated issue work. Authentication and provider operations were handled by the harness connection; Loaf did not receive provider credentials or proxy provider traffic. Journal history, wraps, handoffs, ideas, sparks, reports, and remote continuity were not migrated.
```
