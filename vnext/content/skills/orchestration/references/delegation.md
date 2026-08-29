# Delegation Contract

Delegation is a bounded execution technique. It does not mint work, change the live definition, or transfer the orchestrator's responsibility to verify results.

## Task Packet

Every delegated task includes:

- the exact native tracker reference and destination;
- the completion criterion or review lens it serves;
- relevant repository and skill instructions;
- allowed reads, writes, external mutations, and explicit prohibitions;
- owned files or subsystem boundary;
- required tests and evidence;
- whether commits are authorized;
- the expected result format.

## Writer Rule

Only one agent writes an overlapping change at a time. Parallel writers need disjoint ownership that can be verified from paths or subsystem boundaries. When ownership becomes ambiguous, stop one writer before continuing.

## Acceptance Rule

The orchestrator inspects the actual diff, verifies signature and history requirements, reruns relevant tests, and compares the result with the live contract. A review round converges when independent lenses find no new defect class and all earlier findings have an evidence-backed disposition. Reviewers inform the decision; they do not decide by majority.
