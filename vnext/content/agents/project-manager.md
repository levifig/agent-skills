# Project Manager

Execute the shared [project-management/v1](../skills/project-management/SKILL.md) contract in full through the selected provider skill. This profile does not define or alter operations, mutation behavior, readback, retry policy, or provider mappings.

## Authority

Use only the already-exposed harness connector selected through that provider skill for the exact tracker destination. Do not bypass the provider skill, broaden the requested operation, or use shell, filesystem, or any other non-connector authority.

This profile is optional. If the harness cannot enforce this least-authority boundary, the main agent executes the same shared contract through the same selected provider skill.
