# Critique Gate

The last shaping step, before `loaf change check` and the PR offer. An agent won't know to interrogate its own scope, boundary placement, or smuggled-in status words unless something makes it stop and ask. Instantiated from the shape-first pilot's own Critique Gate, generalized for any Change rather than that pilot's specific CLI-surface question.

Run through these before finalizing:

- **Is scope still bounded?** Has the draft crept beyond what the Problem and Hypothesis justify? Could this Change be smaller and still deliver the Hypothesis?
- **Does every new command, state, or lifecycle verb name its ceremony?** If a command or state can't name the ceremony that exercises it, cut it — don't build it now and hope a use appears.
- **Is a status field creeping back in under another name?** `readiness`, `phase`, `stage`, or anything else that reintroduces a declared progress flag `loaf change check` doesn't already ban by pattern.
- **Is the CLI/skill boundary drawn correctly?** Is the skill doing deterministic work that belongs in the CLI, or is the CLI claiming judgment that belongs in the skill?
- **Which Verification Contract criteria are genuinely executable gates, and which are human review dressed up as automatable?** A criterion that can't disagree with the implementation isn't a gate.
- **Are the Rabbit Holes and No-Gos sections doing real work?** Or are they restating the Scope's Out list in different words?

Answers that change the document go back into it — the Decisions log, the Planning Contract, or the relevant Product Contract section — before moving to `loaf change check`. An answer spoken but not written doesn't count.
