# Critique Gate

The last shaping step, before `loaf issue check` and any review offer. An agent won't know to interrogate its own scope, boundary placement, or smuggled-in progress words unless something makes it stop and ask.

Run through these before finalizing:

- **Is this rideable?** Can the named rider use the result today as a complete small product? Does the journey exercise every new machine? What evidence demanded the complexity? Could less deliver the same outcome without reducing integrity? If this is only a wheel, which immediate consuming slice makes it rideable? Use the full [rideable-increment contract](../../foundations/references/rideable-increments.md).
- **Is scope still bounded?** Has the draft crept beyond what the problem statement justifies? Could this issue be smaller and still be verifiable alone and revertible alone?
- **Does every new command, state, or lifecycle verb name its ceremony?** If a command or state can't name the ceremony that exercises it, cut it — don't build it now and hope a use appears.
- **Is a second progress flag creeping into the body?** `readiness`, `phase`, `stage`, or anything else that reintroduces a declared progress flag. Status lives on the issue row (`loaf issue status`). Shaped, covered, and ready are derived by `loaf issue check`. `loaf issue bucket` is a label only and is never read as a constraint.
- **Is the CLI/skill boundary drawn correctly?** Is the skill doing deterministic work that belongs in the CLI, or is the CLI claiming judgment that belongs in the skill?
- **Which criteria are genuinely executable gates, and which are human review dressed up as automatable?** A V-tier criterion needs `--command` (and `--expect` when exit 0 is not enough). A criterion that can't disagree with the implementation isn't a gate — make it H, or rewrite it against an independent source of truth.
- **Is the out-of-scope statement doing real work?** Rabbit holes and no-gos belong there as named exclusions. Restating the problem in different words is not an out-of-scope statement.
- **Did fog graduate, or evaporate?** Every named unknown is parked in create-time `fog`, held in the session register, sitting as a decision child (or sibling) with a sharp question, filed as a new backlog issue (a different problem), or written into the body as a decided answer. Silent deletion is the failure.

Answers that change the issue go back into it — `loaf issue edit` for the body, `loaf issue dod add` / `remove` for criteria, `loaf issue new --ref <child-ref> --parent <parent-ref>` for children — before moving to `loaf issue check`. An answer spoken but not written doesn't count.
