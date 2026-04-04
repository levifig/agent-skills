---
type: brainstorm
title: "Loaf's Orchestration Ceiling — Framework vs Runtime"
created: 2026-04-04T19:47:42Z
status: active
trigger: Repeated failure of prompt-based hooks to guarantee workflow compliance
---

# Loaf's Orchestration Ceiling

## The Problem

Loaf operates as a skill/hook injection layer inside existing AI coding tools. Hooks are prompt-based nudges — they inject text into the model's context, but the model decides what to do with it. This session demonstrated the ceiling repeatedly:

- Session journal nudge fired 5+ times; ignored or deferred each time
- Making the prompt "REQUIRED" and "before responding" didn't change behavior
- The evaluator can detect non-compliance but cannot enforce compliance
- Pre-push advisory hooks block the tool call but can't force the model to satisfy conditions

**The fundamental gap:** Prompt-level enforcement is a contradiction. If the model could always follow prompt instructions perfectly, we wouldn't need enforcement. The whole point of enforcement is to catch when the model doesn't comply — and prompt hooks can't do that mechanically.

## Landscape

Research identified three architectural postures in the current market:

### 1. Wrapper/Multiplexer (Conductor, Commander)
- Wraps existing agent CLIs with workspace management and UI
- Does **not** own the agent loop
- Cannot enforce anything the underlying agent doesn't already enforce
- Value: parallelism (Conductor), unified UI (Commander)

### 2. Agent Loop Owner (OpenAI Codex, OpenClaw)
- Controls the runtime, tool execution, and approval flow
- **Can** mechanically enforce constraints — mediates every tool call
- Trade-off: tight coupling to provider (Codex = OpenAI only) or large surface area (OpenClaw)
- Value: guaranteed compliance, sandbox isolation, approval gates

### 3. Orchestration Overlay (Paperclip)
- Does **not** own the agent loop — agents are external
- Introduces mechanical enforcement **between** invocations: budgets, task checkout, heartbeat protocol, audit logs
- Value: governance without replacing the agent runtime
- Closest to where Loaf sits, but with mechanical enforcement Loaf lacks

## Where Loaf Sits Today

Loaf is **less than a wrapper** and **more than static config**:
- Skills provide knowledge (portable across 6 harnesses)
- Hooks provide lifecycle events (harness-native surfaces)
- CLI provides project management (tasks, specs, sessions, releases)
- But enforcement is purely advisory — the model is the executor, not Loaf

## Options

### A. Stay as Injectable Framework
Optimize hooks/skills as far as they can go. Accept the enforcement ceiling.
- **Pros:** Simple, portable, no new infrastructure
- **Cons:** Can never guarantee workflow compliance; frustration grows as workflows mature
- **Verdict:** Insufficient. The ceiling is the problem.

### B. Orchestration Overlay (Paperclip-adjacent)
Add mechanical enforcement between agent invocations without replacing the harness.
- Heartbeat protocol: agent must check in with Loaf between actions
- Budget/token caps with atomic enforcement
- Task checkout with pre/post-condition validation
- Audit log of all agent decisions with compliance scoring
- **Pros:** Works with any harness; enforcement is mechanical, not prompt-based
- **Cons:** Enforcement granularity is coarse (between invocations, not within them); requires harness cooperation or a monitoring mechanism
- **Key question:** How does Loaf observe and gate agent behavior without owning the loop? File watching? MCP? Hook callbacks?

### C. Agent SDK Application (Own the Loop)
Use Claude SDK / multi-provider SDKs to build Loaf as the agent runtime.
- Loaf becomes the orchestrator: receives user intent, plans actions, calls models, validates responses, executes tools
- Skills become the knowledge layer (unchanged)
- Hooks become mechanical validators (guaranteed execution)
- Workflow rules enforced by code, not prompts
- **Pros:** Full enforcement; can implement approval gates, validation passes, journal auto-capture mechanically
- **Cons:** Replaces Claude Code/Cursor/etc. rather than augmenting them; massive scope increase; UX challenge (terminal, IDE, web?)
- **Key question:** Do users want another coding agent, or do they want their existing agent to be better governed?

### D. Sidecar Process (Hybrid)
A lightweight daemon that runs alongside any harness, monitoring and enforcing.
- Watches conversation state (via MCP, file events, or harness hooks)
- Validates compliance after each agent response (journal updated? tests run? decisions logged?)
- Can inject corrections via MCP tool calls or file mutations
- Can block responses via pre-tool hook integration
- **Pros:** Non-invasive; works with existing harnesses; enforcement is mechanical
- **Cons:** Monitoring fidelity depends on harness integration; can't enforce within a single model response
- **Key question:** Is MCP a sufficient observation/control surface, or do we need deeper harness integration?

### E. Protocol Layer (Standard + Reference Implementation)
Define a standard agent governance protocol. Loaf becomes the spec + reference impl.
- Workflow compliance as a protocol: agents declare capabilities, Loaf validates claims
- Any harness that implements the protocol gets guaranteed compliance
- Think: what MCP did for tool access, but for workflow governance
- **Pros:** Maximum portability; ecosystem play; could become industry standard
- **Cons:** Adoption requires harness buy-in; chicken-and-egg problem; slow to gain traction
- **Key question:** Is there enough demand for a governance protocol to justify the standardization effort?

## The Sweet Spot

The user's intuition ("halfway between Conductor/Commander and Paperclip/OpenClaw") points toward **B or D** — something that adds mechanical enforcement without replacing the entire agent stack.

The most promising path might be **D (Sidecar) with elements of B (Orchestration Overlay)**:

1. A `loaf daemon` process that runs alongside Claude Code/Cursor/Codex
2. Connected via MCP or file-watching to observe agent behavior
3. Validates workflow compliance mechanically (not via prompts)
4. Can gate actions: "journal not updated → block next commit"
5. Exposes a dashboard for visibility into compliance state
6. Skills and hooks remain the knowledge layer — the sidecar is the enforcement layer

This preserves Loaf's cross-harness portability while adding the mechanical guarantees that prompt-based hooks cannot provide.

## Sparks

- **loaf daemon** — a persistent sidecar process for workflow enforcement, connected via MCP
- **Compliance scoring** — rate agent sessions by how well they followed workflow rules (journal, tests, decisions)
- **Governance protocol** — an open standard for agent workflow compliance, like MCP but for process
- **Agent SDK fallback** — if sidecar proves insufficient, build a reference orchestrator using Claude SDK as proof-of-concept
- **Heartbeat-gated execution** — agents must check in with Loaf between significant actions; Loaf validates preconditions before allowing continuation
- **Visual compliance dashboard** — real-time view of session state, decisions logged, rules satisfied/violated
- **Multi-model reviewer** — Haiku validates Opus output for compliance before it reaches the user

## Open Questions

1. Is MCP a sufficient observation/control surface for a sidecar, or is it too limited?
2. Would users accept a daemon requirement, or does it need to be zero-install?
3. Should the sidecar be language-agnostic (protocol-based) or tightly coupled to Loaf's TypeScript CLI?
4. How does this interact with SPEC-024 (harness-native surface model) — is "governance surface" another row in the matrix?
5. Could the Claude Agent SDK serve as both the enforcement runtime AND remain compatible with Claude Code (via MCP or subagent patterns)?
