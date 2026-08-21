package cli

import (
	"encoding/json"
	"strings"

	_ "embed"
)

//go:embed amp_prompts/medium.md
var nativeAmpMediumPrompt string

//go:embed amp_prompts/ultra.md
var nativeAmpUltraPrompt string

//go:embed amp_prompts/overlay.md
var nativeAmpSharedOrchOverlay string

//go:embed amp_prompts/medium_overlay.md
var nativeAmpMediumOverlay string

//go:embed amp_prompts/ultra_overlay.md
var nativeAmpUltraOverlay string

func nativeAmpJSONString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func nativeAmpDelegatedAgents() string {
	body := `// ─────────────────────────────────────────────────────────────────────────────
// Loaf Medium / Loaf Ultra orchestrators and pinned delegates
// ─────────────────────────────────────────────────────────────────────────────
//
// Selectable modes are loaf-medium and loaf-ultra only. Grok, Luna, and the
// pinned Sol oracle are tools, not picker modes. Built-in Amp medium cannot be
// rewritten; operators use Loaf Medium instead. After tool invocation, pinned
// models, reasoning, features, and least-authority tool lists are exact.
// Unavailable pins fail closed with an actionable diagnostic — no silent
// fallback, no local fallback, no model substitution, and no capability
// expansion.
//
// Workdir is routing context, not a sandbox. Isolation requires a Loaf-started
// isolated worktree or an appropriate runner. Luna consumes a nonempty
// caller-prepared diff/snapshot; Loaf itself must not execute Git to prepare it.
// After install/upgrade, preflight the modes, pinned models, and tool names.
// Operators may remove leftover picker-mode prototypes such as
// ~/.config/amp/plugins/delegated-agents.ts to prevent duplicate tool names.
// Loaf never edits or deletes that file or unrelated Amp plugins.

const GROK_MODEL = 'xai/grok-4.6';
const LUNA_MODEL = 'openai/gpt-5.6-luna';
const SOL_MODEL = 'openai/gpt-5.6-sol';
const GROK_IMPL_TOOLS = [
  'Read',
  'finder',
  'shell_command',
  'shell_command_status',
  'apply_patch',
  'view_media',
];
const LUNA_REVIEW_TOOLS = [
  'Read',
  'finder',
];
const SOL_ORACLE_TOOLS = [
  'Read',
  'finder',
];
const ULTRA_TOOL_NAMES = [
  'finder',
  'shell_command',
  'shell_command_status',
  'create_file',
  'edit_file',
  'web_search',
  'read_web_page',
  'portal_observe',
  'portal_control',
  'read_thread',
  'find_thread',
  'list_agent_modes',
  'list_runners',
  'create_thread',
  'thread_interact',
  'wait_for_threads',
  'download_thread_file',
  'upload_thread_file',
  'notepad',
  'skill',
  'load_plugin',
  'reload_plugins',
  'reload_skills',
  'oracle',
  'librarian',
  'Task',
  'view_media',
  'painter',
  'public_artifact_url',
  'thread_file_url',
  'read_mcp_resource',
  'get_current_user_identity',
  'list_workspace_members',
  'find_shared_plugins_and_skills',
  'send_email',
  'slack_write',
  'slack_read',
  'get_schedule',
  'set_schedule',
  'update_schedule',
  'clear_schedule',
  'create_slack_trigger',
  'x_read',
  'x_reply',
  'mcp__*',
];
const ORCHESTRATOR_TOOLS = [
  'Read',
  ...ULTRA_TOOL_NAMES,
  'delegate_grok_implementation',
  'delegate_luna_review',
  'consult_oracle',
];

const IMPLEMENT_INSTRUCTIONS = %%BT%%You are an implementation specialist. Work on one bounded, fully specified coding issue at a time.

Read the repository guidance and issue contract before editing. Implement the smallest complete solution, add focused regression tests, run the issue-specific checks and relevant broader checks, and report changed files, verification results, assumptions, and blockers. Preserve unrelated work. Do not commit, push, merge, rewrite history, or modify shared external state. Workdir is routing context, not a sandbox.%%BT%%;

const REVIEW_INSTRUCTIONS = %%BT%%You are an independent senior code reviewer. Review the requested implementation against its issue contract and repository guidance.

Remain read-only: do not edit files, apply patches, commit, push, merge, rewrite history, or modify issue and project state. Inspect the caller-prepared diff and relevant surrounding code, run only non-mutating checks when useful, and prioritize correctness, data integrity, concurrency, security, tenant isolation, regressions, and missing tests over style. Return findings ordered by severity with precise file and line references, explain the failing sequence or invariant, and say explicitly when no actionable findings remain. Workdir is routing context, not a sandbox.%%BT%%;

const ORACLE_INSTRUCTIONS = %%BT%%You are a pinned second-opinion specialist (GPT-5.6 Sol, high reasoning). You do not implement and you do not own the user's thread.

Stay read-only. Answer the caller's unresolved question using the supplied evidence and only the extra file reads you need. Return: the verdict, the invariant or failing sequence, evidence with file paths, the strongest alternative, and what would reverse your conclusion. If the evidence is insufficient, say what is missing instead of guessing. Do not edit files, run mutating commands, commit, push, or delegate further.%%BT%%;

const MEDIUM_PROMPT = %%MEDIUM_PROMPT%% as string;
const GROK_46_PROMPT = %%ULTRA_PROMPT%% as string;
const SHARED_ORCH_OVERLAY = %%SHARED_OVERLAY%% as string;
const MEDIUM_OVERLAY = SHARED_ORCH_OVERLAY + %%MEDIUM_OVERLAY%%;
const ULTRA_OVERLAY = SHARED_ORCH_OVERLAY + %%ULTRA_OVERLAY%%;

function requireNonEmptyString(input: unknown, name: string): string {
  if (typeof input !== 'string' || input.trim().length === 0) {
    throw new Error(%%BT%%${name} must be a non-empty string%%BT%%);
  }
  return input;
}

function requireWorkdir(input: unknown): string {
  const requested = requireNonEmptyString(input, 'workdir');
  if (!isAbsolute(requested)) {
    throw new Error('workdir must be an absolute path');
  }
  const workdir = realpathSync(requested);
  if (!statSync(workdir).isDirectory()) {
    throw new Error('workdir must be an existing directory');
  }
  return workdir;
}

function assertFiniteAllowlist(tools: readonly string[], role: string): void {
  if (tools.length === 0) {
    throw new Error(%%BT%%${role} requires a finite local tool allowlist%%BT%%);
  }
  for (const tool of tools) {
    if (tool === 'all' || tool.includes('*') || tool.startsWith('mcp' + '__')) {
      throw new Error(%%BT%%${role} must not expand capabilities to ${tool}%%BT%%);
    }
  }
}

function delegateCompatibilityFailure(role: string, model: string, tools: readonly string[], cause?: unknown): Error {
  const detail = cause instanceof Error ? cause.message : cause ? String(cause) : '';
  return new Error([
    %%BT%%${role} compatibility failure: pinned model ${model} or required tools [${tools.join(', ')}] are unavailable.%%BT%%,
    detail,
    'Do not substitute a model, reasoning level, broader capability set, or local execution by the orchestrating model.',
    'There is no silent fallback and no local fallback.',
    'Run %%BT%%amp plugins show-agent-options --json%%BT%% and confirm the pinned model id and each required built-in tool name before retrying.',
  ].filter(Boolean).join(' '));
}

function assertDelegateCompatibility(role: string, model: string, tools: readonly string[]): void {
  assertFiniteAllowlist(tools, role);
}

function createPinnedAgent(amp: PluginAPI, role: string, model: string, tools: readonly string[], config: Parameters<PluginAPI['createAgent']>[0]): ReturnType<PluginAPI['createAgent']> {
  try {
    return amp.createAgent(config);
  } catch (cause) {
    throw delegateCompatibilityFailure(role, model, tools, cause);
  }
}

function registerPinnedAgentMode(amp: PluginAPI, role: string, model: string, tools: readonly string[], definition: Parameters<PluginAPI['registerAgentMode']>[0]): void {
  try {
    amp.registerAgentMode(definition);
  } catch (cause) {
    throw delegateCompatibilityFailure(role, model, tools, cause);
  }
}

function registerPinnedTool(amp: PluginAPI, role: string, model: string, tools: readonly string[], definition: Parameters<PluginAPI['registerTool']>[0]): void {
  try {
    amp.registerTool(definition);
  } catch (cause) {
    throw delegateCompatibilityFailure(role, model, tools, cause);
  }
}

async function runPinnedAgent(
  agent: ReturnType<PluginAPI['createAgent']>,
  role: string,
  model: string,
  tools: readonly string[],
  prompt: string,
  ctx: { thread: { id: string } },
): Promise<{ threadID: string; text: string }> {
  try {
    return await agent.run(prompt, {
      parentThreadID: ctx.thread.id,
      executor: 'local',
      timeoutMs: 3_600_000,
    });
  } catch (cause) {
    throw delegateCompatibilityFailure(role, model, tools, cause);
  }
}

function registerDelegatedAgents(amp: PluginAPI): void {
  if (typeof amp.createAgent !== 'function' || typeof amp.registerAgentMode !== 'function' || typeof amp.registerTool !== 'function') {
    throw new Error([
      'Amp plugin API is missing createAgent, registerAgentMode, or registerTool.',
      'Do not substitute a model, reasoning level, broader capability set, or local execution by the orchestrating model.',
      'There is no silent fallback and no local fallback.',
      'Upgrade Amp, then run %%BT%%amp plugins show-agent-options --json%%BT%% to confirm the current model/tool surface.',
    ].join(' '));
  }

  assertDelegateCompatibility('grok-impl', GROK_MODEL, GROK_IMPL_TOOLS);
  assertDelegateCompatibility('luna-review', LUNA_MODEL, LUNA_REVIEW_TOOLS);
  assertDelegateCompatibility('sol-oracle', SOL_MODEL, SOL_ORACLE_TOOLS);

  const grokImplementer = createPinnedAgent(amp, 'grok-impl', GROK_MODEL, GROK_IMPL_TOOLS, {
    name: 'grok-implementation-agent',
    model: 'xai/grok-4.6',
    reasoningEffort: 'high',
    features: ['fast'],
    instructions: IMPLEMENT_INSTRUCTIONS,
    tools: GROK_IMPL_TOOLS,
    display: { label: 'Grok Implement', color: '#0ea5e9' },
  });

  const lunaReviewer = createPinnedAgent(amp, 'luna-review', LUNA_MODEL, LUNA_REVIEW_TOOLS, {
    name: 'luna-review-agent',
    model: 'openai/gpt-5.6-luna',
    reasoningEffort: 'xhigh',
    instructions: REVIEW_INSTRUCTIONS,
    tools: LUNA_REVIEW_TOOLS,
    display: { label: 'Luna Review', color: '#8b5cf6' },
  });

  const solOracle = createPinnedAgent(amp, 'sol-oracle', SOL_MODEL, SOL_ORACLE_TOOLS, {
    name: 'sol-oracle-agent',
    model: 'openai/gpt-5.6-sol',
    reasoningEffort: 'high',
    instructions: ORACLE_INSTRUCTIONS,
    tools: SOL_ORACLE_TOOLS,
    display: { label: 'Sol Oracle', color: '#f59e0b' },
  });

  const mediumOrch = createPinnedAgent(amp, 'loaf-medium', SOL_MODEL, ['Read'], {
    name: 'loaf-medium',
    model: 'openai/gpt-5.6-sol',
    reasoningEffort: 'medium',
    instructions: MEDIUM_PROMPT + MEDIUM_OVERLAY,
    tools: ORCHESTRATOR_TOOLS,
    display: { label: 'Loaf Medium', color: '#eab308' },
  });

  const ultraOrch = createPinnedAgent(amp, 'loaf-ultra', SOL_MODEL, ['Read'], {
    name: 'loaf-ultra',
    model: 'openai/gpt-5.6-sol',
    reasoningEffort: 'xhigh',
    instructions: GROK_46_PROMPT + ULTRA_OVERLAY,
    tools: ORCHESTRATOR_TOOLS,
    display: { label: 'Loaf Ultra', color: '#ea580c' },
  });

  registerPinnedAgentMode(amp, 'loaf-medium', SOL_MODEL, ['Read'], {
    key: 'loaf-medium',
    label: 'Loaf Medium',
    description: 'Loaf Medium orchestrator: GPT-5.6 Sol medium plans and decides; Grok 4.6 high+fast implements; Luna xhigh reviews; pinned Sol-high oracle.',
    color: '#eab308',
    agent: mediumOrch.definition,
  });

  registerPinnedAgentMode(amp, 'loaf-ultra', SOL_MODEL, ['Read'], {
    key: 'loaf-ultra',
    label: 'Loaf Ultra',
    description: 'Loaf Ultra orchestrator: Ultra harness, GPT-5.6 Sol xhigh plans and decides; Grok implements; Luna reviews; pinned Sol-high oracle.',
    color: '#ea580c',
    agent: ultraOrch.definition,
  });

  registerPinnedTool(amp, 'grok-impl', GROK_MODEL, GROK_IMPL_TOOLS, {
    name: 'delegate_grok_implementation',
    title: 'Delegate Grok implementation',
    description:
      'REQUIRED default for implementation in Loaf Medium and Loaf Ultra. Delegate one bounded coding issue to Grok 4.6 (high reasoning, Fast). Fail closed: do not implement in the orchestrator thread if this tool errors. Provide the complete issue contract, a Loaf-started isolated worktree or appropriate runner path, constraints, and verification commands. Built-in Amp medium cannot be rewritten; use Loaf Medium. After invocation, Grok 4.6 high+fast is exact with a finite local coding allowlist. Unavailable delegates fail closed with no silent fallback and no local fallback.',
    inputSchema: {
      type: 'object',
      properties: {
        workdir: {
          type: 'string',
          description: 'Absolute workspace or worktree path where implementation must run. Workdir is routing context, not a sandbox.',
        },
        prompt: {
          type: 'string',
          description:
            'Complete implementation brief including outcome, issue contract, paths, constraints, and verification.',
        },
      },
      required: ['workdir', 'prompt'],
    },
    async execute(input, ctx) {
      assertDelegateCompatibility('grok-impl', GROK_MODEL, GROK_IMPL_TOOLS);
      const workdir = requireWorkdir(input.workdir);
      const brief = requireNonEmptyString(input.prompt, 'prompt');
      const prompt = %%BT%%Workspace: ${workdir}\n\n${brief}%%BT%%;
      const result = await runPinnedAgent(grokImplementer, 'grok-impl', GROK_MODEL, GROK_IMPL_TOOLS, prompt, ctx);
      return %%BT%%Implementation thread: https://ampcode.com/threads/${result.threadID}\n\n${result.text}%%BT%%;
    },
  });

  registerPinnedTool(amp, 'luna-review', LUNA_MODEL, LUNA_REVIEW_TOOLS, {
    name: 'delegate_luna_review',
    title: 'Delegate Luna review',
    description:
      'REQUIRED default for review in Loaf Medium and Loaf Ultra. Delegate an independent read-only review to GPT-5.6 Luna (xhigh). Fail closed: do not substitute an orchestrator review if this tool errors. Provide intent, issue criteria, a nonempty caller-prepared diff/snapshot, and relevant verification evidence. Loaf itself must not execute Git to prepare the snapshot. After invocation, Luna xhigh is exact and read-only. Unavailable delegates fail closed with no silent fallback and no local fallback.',
    inputSchema: {
      type: 'object',
      properties: {
        workdir: {
          type: 'string',
          description: 'Absolute workspace or worktree path containing the diff. Workdir is routing context, not a sandbox.',
        },
        diff: {
          type: 'string',
          description:
            'Complete unstaged and staged diff prepared by the orchestrator. Include untracked file contents when relevant. Caller-prepared only; do not ask Luna or Loaf to run Git.',
        },
        prompt: {
          type: 'string',
          description:
            'Complete review brief including intended behavior, issue criteria, worktree path, and checks already run.',
        },
      },
      required: ['workdir', 'diff', 'prompt'],
    },
    async execute(input, ctx) {
      assertDelegateCompatibility('luna-review', LUNA_MODEL, LUNA_REVIEW_TOOLS);
      const workdir = requireWorkdir(input.workdir);
      const brief = requireNonEmptyString(input.prompt, 'prompt');
      const diff = requireNonEmptyString(input.diff, 'diff');
      const prompt = %%BT%%Workspace: ${workdir}\n\n${brief}\n\nDiff supplied by the delegating agent:\n${diff}%%BT%%;
      const result = await runPinnedAgent(lunaReviewer, 'luna-review', LUNA_MODEL, LUNA_REVIEW_TOOLS, prompt, ctx);
      return %%BT%%Review thread: https://ampcode.com/threads/${result.threadID}\n\n${result.text}%%BT%%;
    },
  });

  registerPinnedTool(amp, 'sol-oracle', SOL_MODEL, SOL_ORACLE_TOOLS, {
    name: 'consult_oracle',
    title: 'Consult Sol oracle',
    description:
      'Pinned second-opinion oracle: GPT-5.6 Sol at high reasoning, read-only. Prefer this over the built-in oracle tool. Call after you have read the relevant code, with one unresolved high-impact question plus evidence. Do not use for local search (Finder), external code (Librarian), implementation (Grok), or diff review (Luna). Fail closed with no silent fallback and no local fallback.',
    inputSchema: {
      type: 'object',
      properties: {
        question: {
          type: 'string',
          description: 'The single unresolved question Oracle must answer.',
        },
        evidence: {
          type: 'string',
          description:
            'What you already know: intended behavior, constraints, files, snippets, failing sequence, candidate theories, and what you ruled out.',
        },
      },
      required: ['question', 'evidence'],
    },
    async execute(input, ctx) {
      assertDelegateCompatibility('sol-oracle', SOL_MODEL, SOL_ORACLE_TOOLS);
      const question = requireNonEmptyString(input.question, 'question');
      const evidence = requireNonEmptyString(input.evidence, 'evidence');
      const prompt = %%BT%%Unresolved question:\n${question}\n\nEvidence collected by the orchestrator:\n${evidence}%%BT%%;
      const result = await runPinnedAgent(solOracle, 'sol-oracle', SOL_MODEL, SOL_ORACLE_TOOLS, prompt, ctx);
      return %%BT%%Oracle thread: https://ampcode.com/threads/${result.threadID}\n\n${result.text}%%BT%%;
    },
  });
}
`
	replacer := strings.NewReplacer(
		"%%BT%%", "`",
		"%%MEDIUM_PROMPT%%", nativeAmpJSONString(nativeAmpMediumPrompt),
		"%%ULTRA_PROMPT%%", nativeAmpJSONString(nativeAmpUltraPrompt),
		"%%SHARED_OVERLAY%%", nativeAmpJSONString(nativeAmpSharedOrchOverlay),
		"%%MEDIUM_OVERLAY%%", nativeAmpJSONString(nativeAmpMediumOverlay),
		"%%ULTRA_OVERLAY%%", nativeAmpJSONString(nativeAmpUltraOverlay),
	)
	return replacer.Replace(body)
}
