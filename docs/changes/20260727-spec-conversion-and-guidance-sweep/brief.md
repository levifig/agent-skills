<!-- Captured promise carrier for target_release 2.0.0 — materializes the live
     release-after promise from journal-reliability-foundation. Shape arrives
     when the sweep (or its successor) is ready to execute. -->

# Spec conversion and guidance sweep

## Original ask

Carry the 2.0.0 release promise formerly held by `release-after: spec-conversion-and-guidance-sweep` on `journal-reliability-foundation`. Declaring `target_release: 2.0.0` opts this change into the strong gate: stable 2.0.0 cannot cut until this carrier is converted (if needed), executed at flip grade, and receipt-verified.

## Context

Materialized by change-work-model TASK-004 atomically with lineage freeze-replay retirement. Prerelease flows regardless.

## Notes

Captured, not shaped. Do not treat this brief as a pseudo-shape.
