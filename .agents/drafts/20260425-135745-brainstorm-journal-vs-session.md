---
title: "Brainstorm: Journal/session reframe → Loaf as multi-surface platform"
type: brainstorm
created: 2026-04-25T13:57:45Z
updated: 2026-04-27T09:53:40Z
status: active
tags:
  - architecture
  - sessions
  - journal
  - cross-harness
  - memory
  - platform
  - runtime
  - acp
  - multi-surface
  - mobile
related:
  - .agents/drafts/20260404-194742-brainstorm-loaf-orchestration-ceiling.md
  - .agents/sessions/20260425-144550-session.md
  - SPEC-020 (cross-harness research, shipped)
  - SPEC-023 (skills backend abstraction, drafting)
  - SPEC-024 (harness-native surface model)
---

# Brainstorm: Journal/session reframe → Loaf as multi-surface platform

**Started:** 2026-04-25 (journal/session decoupling)
**Expanded:** 2026-04-27 (platform synthesis)
**Status:** Active — unresolved decisions on substrate commitment and timeline

## TL;DR for second-opinion readers

This brainstorm started narrowly — *should we decouple Loaf's session model from `claude_session_id` and shift to daily journal files?* — and through several rounds of conversation the question opened up substantially. The current synthesis:

1. **The journal/session question is a symptom, not the disease.** The disease is that Loaf doesn't own the conversation surface, so it can't capture or enforce mechanically. Today's `loaf session log` discipline is a workaround.
2. **Journals are primarily for AI consumption** (context, auditability, queryability) — not human curation. The agent should query them, not author them. Capture should be mechanical, harness-driven.
3. **The April 2026 "orchestration ceiling" brainstorm hit the same wall from the other side** (enforcement instead of capture). Both converge on: Loaf needs to own enough of the runtime to stop relying on agent discipline.
4. **The long-term vision is multi-surface** — GUI, eventually mobile, parallel agents, multiplexed sessions resumable across devices. This is bigger than "build on Pi"; it is *Loaf-as-platform*.
5. **Zed-with-ACP-launching-Claude-Code is operational today.** This collapses the timeline: a substrate exists *now* for mechanical observation, not just in some future Pi build. Pi/custom remain the longer-arc options for full multiplex/mobile.
6. **Revised recommendation:** don't refactor toward daily-journal files. Do the minimum to relieve current pain (decouple `claude_session_id` field; redefine session boundary to be work-driven, not conversation-thread-driven). Move strategic energy to runtime substrate exploration and a "Loaf as platform" architecture document.

## Session Journey

The brainstorm evolved across multiple sessions and days. Each shift came from a user reframe that made earlier conclusions visibly insufficient:

| Phase | Trigger | Conclusion of phase |
|-------|---------|--------------------|
| **Phase 1 — File boundary question** (2026-04-25) | "Decouple session from `claude_session_id`; daily files" | Direction II: daily journal primary, session as frontmatter manifest |
| **Phase 2 — Pi+ACP destination raised** | "If Pi was the harness, do we need a journal? Could sanitized logs work?" | The journal is three roles (index, distillation, discipline); Pi+ACP makes L1 free; question becomes about discipline more than artifact |
| **Phase 3 — Mechanical-capture reframe** | "Journals are primarily for AI; harness handles creation; agent only queries" | Discipline argument falls away. Architecture C/D emerge: harness logs as substrate, Loaf as thin index/ledger |
| **Phase 4 — April brainstorm convergence** | "Decisions go to ADRs/KB. This connects to the orchestration-ceiling brainstorm." | Both brainstorms hit the same wall: Loaf can't be mechanical because it doesn't own the loop. Pi/Zed make C-style "own the loop" tractable |
| **Phase 5 — Vision and Zed-today** | "Zed has ACP today; long-term wants GUI + mobile + parallel + multiplex" | Loaf-as-platform synthesis. Don't optimize the bridge artifact; energy goes to runtime substrate |

The conclusion of Phase 1 is *preserved* below for the trail, but is **superseded** by the Phase 5 synthesis.

---

# Phase 1 — File boundary question (preserved as history)

## Original framing

Started from a concrete observation: `claude_session_id` lives in session frontmatter and shapes the file's identity. That coupling is fine for Claude Code, but it leaks the harness's runtime concept into Loaf's persistent record. Cross-harness work and a future custom harness amplify the cost.

Quick sweep of current state surfaced reinforcing evidence:
- 60 archived session files plus 5 active — meaningful accumulation, no obvious time-based grouping
- One session today (a658b2e8) had 3 stop/resume cycles across two conversations on the same calendar day — the file's identity drifted from "a unit of work" to "a Claude Code conversation thread"
- Most users (and journals everywhere — Obsidian, Logseq, Roam) work in *days*, not *conversations*
- The compact entry format `[YYYY-MM-DD HH:MM] type(scope): description` is already harness-agnostic; only the *file boundary* is harness-coupled

The reframe in Phase 1: **the current session model conflates two distinct concepts.**

| Concept | Nature | Lifetime | Owned by |
|---------|--------|----------|----------|
| Conversation context | Runtime, ephemeral | Per AI conversation | Harness |
| Work continuity | Persistent, addressable | Project-long | Loaf |

Decoupling these is the actual unlock; daily-file-vs-session-file is just one consequence.

## Dimensions of choice (Phase 1 framework)

The proposal touched at least four orthogonal axes. Worth treating them separately so the recommendation could mix-and-match.

### Axis 1 — File boundary

| Option | Description | Best for | Worst for |
|--------|-------------|----------|-----------|
| **A. Pure daily** | One file per calendar day: `.agents/journal/2026-04-25.md` | Continuity across conversations; low-friction recall; aligns with daily-notes mental model | Long single-topic projects (one file holds many specs); cross-midnight sessions |
| **B. Pure session** *(today)* | One file per harness session | Atomic conversation handoff; clear scope | Cross-harness identity; multi-conversation days; resume churn |
| **C. Topic/spec-keyed** | One file per spec or topic: `.agents/journal/SPEC-024.md` | "Show me everything about X"; long initiatives | Daily review; ad-hoc cross-cutting work |
| **D. Continuous + rotation** | Single growing log, rotated weekly/monthly | Pure append-only ledger; minimal naming | Discoverability; review |
| **E. Hybrid: daily primary + session snapshots** | Daily journal owns content; session "manifests" point at journal slices | Best of A + B; handoff still possible | Two artifact types to maintain |

### Axis 2 — Entry format

| Option | Description | Tradeoff |
|--------|-------------|----------|
| **a. Keep compact inline** *(today's format)* | `[YYYY-MM-DD HH:MM] type(scope): description` | Grep-friendly, harness-agnostic, low ceremony — already proven |
| **b. Markdown-section style** | H2/H3 sections per topic, prose entries | Closer to Obsidian daily notes; richer but heavier |
| **c. YAML frontmatter blocks per entry** | Each entry a YAML block with structured fields | Most queryable; loses readability |
| **d. Mixed** | Compact ledger section + prose reflections section | Ledger keeps audit trail; reflections accommodate end-of-day summaries |

### Axis 3 — Session-ID tracking (multi-harness)

| Option | Description | Tradeoff |
|--------|-------------|----------|
| **i. Tag in start entry** | `session(start): === STARTED === (claude-7b99211f, claude-code)` — IDs live in journal entries | No new artifact; grep-able; harness self-identifies |
| **ii. Separate registry** | `.agents/state/sessions.json` with rows for every session ever | Fast lookup; canonical truth; another file to maintain |
| **iii. Frontmatter manifest per day** | Daily file frontmatter lists `sessions:` array | Discoverable per day; works without registry |
| **iv. None** | Don't track at all; harness IDs are throwaway | Simplest; loses ability to correlate with harness logs |

### Axis 4 — Continuity / handoff mechanism

| Option | Description | Tradeoff |
|--------|-------------|----------|
| **w. Current State doc** | `.agents/state/current.md`, rewritten at session end | Single source of "where am I now"; needs explicit update |
| **x. Last-N-entries scan** | Resume reads tail of today's journal | Always fresh; no extra artifact; weak when picking up next morning |
| **y. Session snapshot** | Session-end produces `.agents/snapshots/{id}.md` distilled view | Best handoff fidelity; revives the artifact we're trying to remove |
| **z. Spec/task-driven** | Continuity comes from active spec + active tasks, not session | Aligns with work-as-spec model; weakest for ad-hoc work |

## Three composite directions (Phase 1, superseded)

### Direction I — "Daily journal, sessions vanish"
**Mix:** A + a + i + w

- `.agents/journal/2026-04-25.md` is the only persistent record
- Compact inline entries unchanged; harness session ID lives in `session(start)` text
- `.agents/state/current.md` (separately written, not session-scoped) handles handoff
- Session files retired entirely

**Pros:** clean, harness-agnostic, minimal artifact sprawl, exactly mirrors daily-notes idiom
**Cons:** loses "this conversation produced these entries" boundary; cross-midnight work splits across files; aggressive migration

### Direction II — "Daily primary, sessions as manifest" — *was Phase 1's recommendation; superseded by Phase 5*
**Mix:** E + a/d + iii + w + optional y

- `.agents/journal/2026-04-25.md` holds *all* entries (the source of truth)
- Frontmatter lists sessions touching that day with harness ID + range
- `session(start/stop)` entries continue to mark boundaries inline
- Session is **runtime-only**: no per-session file is written; if needed, a snapshot is *derived* on demand by slicing the daily journal
- `.agents/state/current.md` updated on session-stop for fast handoff

**Pros:** preserves session boundary information without per-session files; daily file is the canonical artifact; aligns with Obsidian/Logseq idioms; trivially supports any future harness
**Cons:** requires CLI work to derive session snapshots on demand; slightly more frontmatter discipline

> **Why superseded:** This direction still optimizes the *artifact* layer (which file format, where it lives) within today's surface (Claude Code + local files). Phases 3–5 reframed the question to be about *who owns conversation capture* and *what runtime serves multi-surface use cases*. Direction II would polish the bridge artifact while the destination has different requirements entirely.

### Direction III — "Sessions stay, files rename"
**Mix:** B + a + iii + w (minimal change)

- Rename `.agents/sessions/` → `.agents/journal/` but keep one file per session
- Drop `claude_session_id` as a privileged field; move into a generic `harness_session: { id, kind }` block
- Defer daily-file model

**Pros:** smallest blast radius; honors "decouple from claude_session_id" without restructuring
**Cons:** doesn't actually move to daily files; punts the harder choice; future custom harness still inherits the conversation-thread artifact model

---

# Phase 2 — Pi+ACP destination

**Question raised:** *If Pi was the harness, do we need a journal at all? Could sanitized full conversation logs serve directly?*

This unlocked a useful framing — the journal serves three separable roles, only one of which is intrinsic to authoring:

| Role | What it does | Replaceable by sanitized log? |
|------|--------------|-------------------------------|
| **Index** | Makes past content findable by typed event (`decision`, `spark`, `block`) | Yes, with good search/embeddings |
| **Distillation** | Compresses verbose reasoning into terse outcomes | Yes (LLM extraction post-hoc, with quality cost) |
| **Discipline** | Forces the AI to *name* a decision discretely | **No** — this is the act, not the artifact |

The hierarchy that emerged:

```
L1: Full conversation log         (Pi-captured, ground truth, ~50KB/hour)
L2: Sanitized transcript          (derived from L1, human-readable, ~15KB/hour)
L3: Journal/digest entries        (curated/extracted, ~1KB/hour)
```

For "what was discussed" → sanitized works.
For "did we decide X" → sanitized works if you read it.
For "list all auth decisions across 6 months" → sanitized only works if indexed; journal wins on cost/speed/precision.
For "load recent context into a new AI session" → journal wins by ~15× on token cost.

In a Pi+ACP world, **L1 becomes free**. Pi owns every conversation across every backend (Claude Code, Codex, Amp), so the substrate exists whether or not we build on top of it. The real question becomes: **how much of L2/L3 is worth materialising vs. computing on demand?**

Phase 2 leaning: discipline is the journal's strongest defense; everything else is implementation choice. But this leaning didn't survive Phase 3.

---

# Phase 3 — Mechanical capture reframe

**User reframe:** *Journals are primarily for AI. Humans will query the AI. Goal is for AI to have (1) context, (2) auditability, (3) queryability. As mechanical as possible — harness handles creation; agent only queries.*

This reframe **collapses the discipline argument**. If creation is mechanical, the journal isn't a forcing function — it's a data substrate. Different problem.

The remaining roles for the journal under this framing are all **artifact roles**:

| Role | What the AI needs | Mechanical-capture-compatible? |
|------|-------------------|-------------------------------|
| **Context** | Compact resume material on session start | Yes — harness can summarize/excerpt |
| **Auditability** | Persistent record for "what was decided when?" | Yes — full log preserves this trivially |
| **Queryability** | Fast typed lookup ("show all auth decisions") | Yes, but requires indexing/extraction |

None of these *require* AI authorship. All can be served by capture-then-query.

## What "mechanical" can mean (spectrum)

| Level | Who creates entries | Tradeoff |
|-------|--------------------|---------|
| **0. Pure log** | Harness saves transcripts only | Zero structure; queryability via search/embeddings |
| **1. Auto-format** | Harness saves transcripts + structural events (session start/stop, commits, file edits) | Cheap; reliable; misses semantic events (decisions, sparks) |
| **2. Heuristic extraction** | Harness post-processes logs with regex/classifiers to extract typed events | Mechanical, no LLM, pattern-brittle |
| **3. LLM extraction** | Harness runs an LLM digester on closed conversations to produce typed events | High fidelity; needs a model in the loop; non-deterministic |
| **4. Inline typed events** | Agent emits events as a *native message type* (not a separate tool call), harness captures | Borderline-mechanical; survives the "no curation" bar if the events are part of the natural utterance, not metacognitive overhead |

User's "little to no agent involvement" rules out today's `loaf session log` pattern and pushes toward levels 0–3.

## Decisions go elsewhere (user observation)

A key user observation: *decisions, intentional artifacts, and curated content already have proper homes* — ADRs, KB, ideas, sparks register, reports, drafts, specs, tasks. The journal was carrying decision-tracking *as a side effect*. With proper destinations, the journal doesn't need to be a decision archive at all.

This collapses the journal's remaining responsibility to: **chronological/structural ledger** (session boundaries, branch changes, commits, file edits) — small enough to be auto-emitted by hooks today.

| Type | Old home | New home |
|------|----------|----------|
| Decisions | Journal `decision(scope):` entries | ADRs (`docs/decisions/`) or KB |
| Sparks | Journal `spark(scope):` entries | `.agents/ideas/` or sparks register |
| Discoveries | Journal `discover(scope):` entries | KB or session-internal only |
| Tasks/TODOs | Journal `todo(scope):` entries | Tasks system |
| Specs | (links from journal) | `.agents/specs/` |
| Reports | (links from journal) | `.agents/reports/` |
| Drafts | (links from journal) | `.agents/drafts/` |
| Conversation context | Journal entries | Harness logs (raw) |

## New architectures from Phase 3

**Architecture C — "Loaf reads harness logs, writes nothing."**
- Claude Code's JSONL transcripts already exist at `~/.claude/projects/<project>/<session-id>.jsonl`
- Codex has its own log store; Pi will have its own
- Loaf becomes a *query layer* over those native stores
- `loaf journal today` reads today's transcripts from whatever harness ran them
- No `.agents/sessions/` or `.agents/journal/` files at all
- Cross-harness: Loaf knows where each harness stores logs; queries them all
- Mechanical to the maximum: nothing is created by Loaf or the agent

**Architecture D — "Structural ledger only."**
- Loaf captures only what *can't* be derived from conversation logs: commits, branch state, spec status changes, task transitions
- That ledger is tiny — `.agents/journal/YYYY-MM-DD.md` with maybe 5–20 entries per day, all auto-emitted by hooks (post-commit, on `loaf task complete`, etc.)
- Conversation content stays in harness logs; Loaf indexes pointers to it (`see ~/.claude/projects/.../sess-7b9.jsonl#turn-42`)
- Agent doesn't write journal entries; harness hooks do
- Compatible with Pi+ACP: Pi-managed logs replace per-harness logs, ledger format unchanged

Both are radically smaller than today's session model. Both align with "harness captures, agent queries."

---

# Phase 4 — Convergence with the April orchestration-ceiling brainstorm

**Reference:** [`.agents/drafts/20260404-194742-brainstorm-loaf-orchestration-ceiling.md`](20260404-194742-brainstorm-loaf-orchestration-ceiling.md)

The April 2026 brainstorm identified Loaf's *enforcement ceiling*: prompt-based hooks can't guarantee compliance because Loaf doesn't own the loop. Five architectural options were explored:

- **A.** Stay as injectable framework (rejected — ceiling is the problem)
- **B.** Orchestration overlay (Paperclip-adjacent — heartbeat protocol, budgets, audit logs)
- **C.** Agent SDK Application — own the loop (rejected as massive scope)
- **D.** Sidecar process (hybrid — daemon alongside any harness, MCP integration)
- **E.** Protocol layer (governance protocol + reference impl)

April's leaning was **D + B**: a sidecar daemon with mechanical enforcement between invocations.

This brainstorm, viewed from the journal/session angle, hits the **same wall**:

| Brainstorm | Symptom | Underlying issue |
|------------|---------|------------------|
| **April (orchestration ceiling)** | Hooks fire 5+ times, ignored. Pre-push blocks but can't satisfy condition. | Loaf advises; the model executes. Enforcement is impossible. |
| **This brainstorm (journal/session)** | `claude_session_id` leaks. Sessions fragment. Curation requires AI emission. | Loaf can't capture; the harness owns the conversation. Mechanical journaling is impossible. |

April was about **output** (enforcement); this is about **input** (capture). Both reduce to: **Loaf isn't in the loop.**

April's option C ("own the loop via Agent SDK") was rejected as *"massive scope increase."* But Pi-as-substrate or Zed-as-substrate flips that economics: someone else provides the runtime, Loaf becomes a layer *inside* their model. Option C becomes tractable not as "build our own runtime from scratch" but as "build on someone else's runtime."

This is a **real unlock for the April direction** that wasn't visible at the time.

---

# Phase 5 — Vision and Zed-today

**User reveal:** *Zed already has ACP support and can launch Claude Code from within its UI (currently using this). Pi may not yet but could. Long-term vision: GUI, eventually mobile, parallel agents, fully multiplexed sessions resumable across devices.*

Two things changed at once:

1. **Substrate exists today.** Zed-with-ACP is operational. Loaf can plug in *now*, not after Pi is built.
2. **Vision is bigger than "build on Pi."** Mobile + multiplex + parallel implies a backend architecture, not just a different harness.

## Substrate options compared

| Substrate | Style | What it gives Loaf | Tradeoff | Available |
|-----------|-------|-------------------|----------|-----------|
| **Zed + ACP** | GUI/IDE — agent orchestration native to the editor | Mature UI, worktree parallelism, IDE-context for agents, ACP-driven observation | Editor-bound; users must adopt Zed; less freedom on the surface | **Today** |
| **Pi (pi.dev)** | TUI/runtime, "computer for thinking" — agents as OS-level primitives | A runtime we can sit *inside*, not adjacent to. Full conversation capture, mechanical hooks, ACP as the bridge to existing agents | Less mature UX; we shape it; needs ACP build-out | Mid-term |
| **Loaf-as-backend (`loafd`)** | Server/daemon Loaf owns | Full multiplex + mobile + cross-device. Surfaces become clients. | Big architectural commitment; hosting/sync layer; biggest scope | Long-term |

The Pi vs Zed choice is a **UX choice, not an architecture choice.** Both let Loaf own mechanical capture/enforcement. The architectural commitment ("Loaf is a layer inside a runtime, not a sidecar to a harness") is the same either way.

## What the long-term vision implies

Working through "GUI + mobile + parallel + fully multiplexed":

| Implication | Why |
|-------------|-----|
| **Sessions must be runtime-state, not local files.** | A phone resuming a session can't be reading `.agents/sessions/...` on a laptop. |
| **Multiplex implies session-as-handle.** | tmux model: server-owned context; any client attaches; multiple clients can view/contribute simultaneously. |
| **Parallel implies orchestrated worktrees.** | N agents on the same project requires Loaf to mediate worktree assignment, branch coordination, conflict avoidance. |
| **Cross-device implies a backend.** | Phone can't run Claude Code locally; thin ACP client → remote `loafd` orchestrator. |

## The architectural shape

```
┌─────────────────────────────────────────────────────────┐
│ Surfaces (clients)                                       │
│   Zed plugin │ Pi TUI │ Custom GUI │ Mobile │ loaf CLI  │
└─────────────────────────┬───────────────────────────────┘
                          │ ACP (or extension thereof)
┌─────────────────────────▼───────────────────────────────┐
│ loafd — orchestrator / backend                          │
│   Owns: sessions, conversation buffers, agent lifecycle │
│   Speaks ACP to clients; spawns/manages agents          │
│   Persistence: project artifacts + runtime state        │
└─────────────────────────┬───────────────────────────────┘
                          │
       ┌──────────┬───────┴──────┬──────────┐
       ▼          ▼              ▼          ▼
   Claude Code  Codex          Amp        Future agents
```

In this picture:
- **Loaf becomes a backend, not a CLI.** Today's `loaf` CLI becomes one of N clients.
- **`.agents/` directory** keeps holding what's project-state: specs, tasks, KB, ADRs, decisions, drafts.
- **Sessions/journals** move out of `.agents/` and into orchestrator-owned runtime state (DB, sync layer, or both).
- **Skills, hooks, conventions** survive — they're the knowledge/discipline layer that runs *in* the orchestrator.

---

# Insights through the journey

## ★ Insight ─────────────────────────────────────

1. **The current model is a leaky abstraction.** Conversation context (ephemeral, harness-owned) is fused with work continuity (persistent, project-owned). Decoupling these is the actual unlock; daily-file-vs-session-file is just one consequence.

2. **The journal/session question is really a layering question.** With Pi+ACP, L1 (full log) is unavoidable. L2 (sanitized) is cheap. The choice is whether L3 (digest) is a *materialised file*, an *inline annotation in L1*, or a *derived view computed on demand*.

3. **The "do we need journals?" question is really a discipline question.** If the AI emits journal entries, the act of naming a decision *changes the conversation* — it commits, makes the decision discrete. If capture is fully mechanical, that discipline disappears. We have to choose whether we want the discipline or just the artifact.

4. **The discipline argument falls when capture is mechanical.** Once "harness handles capture" is the goal, the journal isn't a forcing function — it's a data substrate. Different problem.

5. **With proper artifacts, the journal almost dissolves.** Once you list the intentional homes — ADRs, KB, ideas, sparks, reports, drafts, specs, tasks — the journal's only remaining job is *chronological/structural*: session boundaries, branch changes, commits, file mutations. Everything semantic has a better home.

6. **The two brainstorms converge on the same answer.** "Sidecar with mechanical enforcement" (April option D) and "harness captures, agent queries" (today's reframe) are the same architectural commitment — Loaf needs to own enough of the conversation surface to observe and react without depending on the agent's discipline. Pi or Zed makes that ownership feasible at acceptable scope.

7. **Pi vs Zed is a UX choice, not an architecture choice.** Both let Loaf own mechanical capture/enforcement. The architectural commitment is the same either way. The choice is about audience and timeline.

8. **The vision implies Loaf-as-platform, not Loaf-as-tool.** Mobile + multiplex + parallel can't be served by a CLI that scaffolds files. They require a backend that owns runtime state and serves multiple surfaces. Once you accept that, the journal/session question becomes "what does the runtime persist, and what does the backend orchestrate?"

9. **Session redefines under multiplex.** Today: session = a Claude Code conversation. Tomorrow: session = a server-owned context any client can attach to (tmux model). This is *the* conceptual shift; everything else is plumbing. Cross-device resume is the killer feature this enables.

10. **Zed-ACP-today provides a free prototype substrate.** We can validate "Loaf observes via ACP, owns nothing about conversation content" without building the backend yet. If that pattern works, scaling it to a `loafd` orchestrator is mostly a deployment question.

11. **The journal/session question dissolves under the platform reframe.** If capture is mechanical and runtime state is backend-owned, the file boundary is a rendering decision, not an architectural one. We've been polishing the wrong layer.

─────────────────────────────────────────────────

---

# Revised recommendation (current synthesis)

**Drop Direction II from Phase 1.** Don't refactor toward daily-journal files. That refactor optimizes for the *current* surface — exactly the surface the long-term vision replaces.

## Do the minimum to remove immediate pain (today)

- Drop `claude_session_id` as a privileged frontmatter field; replace with a generic `harness_session: { id, kind }` that accepts any harness
- Stop letting "new conversation" close and reopen the same session multiple times a day; change session boundary semantics so a Loaf session is a *work session* (user-defined), not a Claude Code conversation thread
- Leave file format and location alone; they'll be replaced
- Keep `loaf session log` as a transitional convenience, knowing it's the bridge — don't entrench it; design *out* of it

## Move strategic energy to runtime substrate

Three concrete possibilities, in order of likely value-per-effort:

| Option | Investment | Payoff |
|--------|-----------|--------|
| **Strategic doc: Loaf-as-platform spec/ADR** | Day | Articulate vision-as-architecture so all future decisions have a North Star |
| **Research spike: Zed-ACP integration** | Days | Validate "Loaf observes mechanically via Zed's ACP" — biggest free prototype available |
| **Prototype: minimal `loafd`** | Weeks | Server-side session state, single client (CLI), proves the architectural shift |

The strategic doc should come first; without it, every refactor decision (including journal/session) is made in the wrong frame.

## What this means concretely for the journal/session question

**What Loaf owns persistently in the new world:**
- Specs, tasks, ADRs, KB, ideas, drafts, reports — all already exist, all keep their homes in `.agents/`
- A thin **structural ledger** of project events: session boundaries, commits, branch changes, file edits attributable to the project. Auto-emitted by hooks. Not a "journal" in the curated sense — a flight recorder
- Pointers/index into the runtime's conversation logs (Pi-owned, Zed-owned, or `loafd`-owned), so AI queries can resolve "what happened recently" without Loaf re-capturing it

**What Loaf does not own:**
- Conversation content (runtime owns it)
- Decision narratives (ADRs/KB)
- Idea capture (`/loaf:idea`, sparks register)
- Discovery semantics (KB)
- TODOs (tasks)

The current `loaf session log "decision(scope): …"` pattern was a workaround for not having proper destinations *and* not having mechanical capture. With proper destinations (already built) and mechanical capture (Pi/Zed/`loafd` substrate), it just goes away.

## The bridge problem

Even if the destination is mechanical, today is Claude Code. The bridge has two layers:

1. **Continue today's emission discipline** as belt-and-suspenders, not source-of-truth — knowing it's the bridge, not the destination. Don't entrench it; design *out* of it.
2. **Treat the destination as: Loaf-on-Zed-today / Loaf-on-Pi-mid-term / Loaf-as-backend long-term.** Architectural decisions today should be evaluated against that destination, not against "best Claude Code experience."

This argues against investing heavily in a daily-journal redesign for today's Claude Code surface. Better to leave it minimally-improved and pour energy into the runtime question.

---

# Open questions (for further exploration / second opinions)

These are the unresolved decisions that the synthesis surfaces. Good prompts for second-opinion reviewers.

## Strategic
1. **Does Loaf commit to becoming a platform** (backend + multi-surface), or stay a CLI/skill framework?
2. **If yes, what's the timeline?** Today's Zed-ACP integration as first step? Or jump straight to `loafd` design?
3. **Where does Pi fit?** As a future client (TUI surface) or as the runtime substrate Loaf builds on?
4. **Is Zed-as-current-substrate good enough to be the validation surface, or is it too constrained** (editor-bound, GUI-only)?
5. **Mobile is long-term — does it constrain backend choices today?** (Cloud-hosted vs self-hosted vs P2P sync)
6. **What's the open-source / commercial / hosted model?** A backend implies hosting; is Loaf still purely OSS, or does this open up a service?

## Architectural
7. **In an ACP-based world, what's the minimum useful "structural ledger"** — what events are worth capturing locally that aren't already in conversation logs?
8. **Does `.agents/` become the "project-state" boundary** (specs, tasks, KB, ADRs only), with everything runtime moving out?
9. **How do parallel agents on the same project coordinate** (worktree assignment, file lock awareness, merge orchestration)?
10. **What's the protocol surface between Loaf and surface clients?** Pure ACP, ACP + extensions, or a different protocol for orchestration concerns?
11. **What about ACP itself — is it a sufficient observation/control surface, or do we need deeper integration?** (April's option D question, still relevant.)

## Tactical / immediate
12. **Verb naming:** keep `loaf session start/stop` or rename now to reflect shifted semantics?
13. **Migration:** leave 60+ archived session files alone; do nothing with them?
14. **Cross-project session continuity:** Is project-scoping the right granularity, or do we eventually need a user-scoped session model (one work session can span projects)?
15. **Resume semantics today:** when an AI agent resumes mid-session in Claude Code, does it read today's full journal, last-N entries, `.agents/state/current.md`, or a derived session-slice?
16. **End-of-day artifacts:** do we want an automatic "daily wrap" that summarises recent activity, or keep wraps session-scoped?

## From Phase 1 (still open if Phase 1 partially survives)
17. **Daily file frontmatter shape:** if any subset of Direction II survives as a transitional artifact, what fields does the frontmatter carry?
18. **Cross-midnight handling:** entry timestamp determines which day's file it lands in?
19. **Index granularity:** when the AI queries "show recent decisions," what does "decision" mean if no one tagged it? Heuristic extraction, LLM digestion, or ACP-typed events later?

---

# Sparks

Speculative byproducts collected through the journey. Not promoted to ideas yet.

## From Phase 1 (file boundary exploration)
- **`loaf journal review`** — daily/weekly read-back of journal entries, possibly with auto-tag extraction, for human review or memory promotion
- **Memory layer on top of journal** — claude-mem-style auto-compaction or claude-remember-style persistent notes as a separate skill that *consumes* daily journal files; the journal stays dumb
- **Harness-prefixed session IDs** — adopt `claude-7b9...`, `codex-cs_abc...`, `loaf-...` prefix scheme so identity is self-describing across systems
- **`spark`/`todo` entries auto-promote** — daily wrap could surface unprocessed sparks/todos for triage to `/idea` or task creation
- **Spec-scoped journal views** — `loaf journal --spec SPEC-024` greps entries with `(spec-024)` scope across all daily files; replaces "show me this spec's history"
- **Pre-compaction librarian role on daily files** — when AI context approaches limit, librarian distills recent days into a compact "context preface" instead of session-scoped distillation
- **Journal as KB substrate** — daily files become first-class addressable nodes in the knowledge base graph, joining specs/tasks/decisions in `loaf kb` queries
- **Session as logical span, not file** — `session_id` becomes a tag attached to a date-range across one or more daily files, queryable but never materialised as its own artifact

## From Phase 5 (platform synthesis)
- **`loafd` orchestrator** — daemon/server with ACP server-side, multi-client, runtime state owner
- **Session-as-tmux-context** — server-owned, attach-from-anywhere, multiplex-friendly
- **Zed-ACP observation pattern** — Loaf as a passive observer of Zed's ACP stream; pure read; validation prototype
- **Strategic doc: "Loaf-as-platform"** — vision/architecture artifact that re-grounds all future decisions; possibly an ADR
- **`.agents/` purity** — explicit declaration of what is and isn't project-persisted state; runtime state moves out
- **Mobile thin client** — ACP-speaking mobile app that connects to remote `loafd`
- **Cross-device session resume** — laptop session continues on phone; same conversation buffer, different surface
- **Parallel-agent coordination protocol** — beyond ACP, an orchestration layer for "agent A is editing file X, agent B should hold off"
- **`loafd` as MCP host** — Loaf orchestrator serves MCP tools to whichever agent surface is active, unifying tool access across surfaces
- **Compliance scoring on conversation logs** — April brainstorm spark, now feasible with mechanical capture: rate sessions on workflow adherence
- **Multi-model reviewer** — Haiku validates Opus output for compliance before reaching the user (revived from April brainstorm)
- **Loaf hosted service** — opt-in cloud-hosted `loafd` for cross-device sync without self-hosting; opens a commercial path
- **Surface-as-skin** — the same session can present differently per surface (terse on mobile, rich on desktop) while sharing one runtime context
- **Journal as Loaf's "git log"** — once mechanical, the journal becomes git-log-equivalent: read by tools, rarely by humans directly, but always available for archeology
