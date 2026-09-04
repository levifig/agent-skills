import type { PluginAPI as DelegationAPI, PluginToolContext, PluginToolDefinition, PluginThread, Subscription, ToolCallEvent } from '@ampcode/plugin';
import { execFile as delegationExecFile } from 'node:child_process';
import { createHash } from 'node:crypto';
import { lstat, realpath } from 'node:fs/promises';
import { dirname as delegationDirname, isAbsolute, relative, resolve, sep } from 'node:path';
import { promisify as delegationPromisify } from 'node:util';

const delegationExec = delegationPromisify(delegationExecFile);
type DelegationCommand = (command: string, args: string[], cwd?: string) => Promise<string>;
type DelegationRole = 'implementation' | 'review';
type DelegationGuard = { role: DelegationRole; root: string; sealed: boolean };
type DelegationToolCall = Pick<ToolCallEvent, 'toolUseID' | 'tool' | 'input'> & { thread: { id: string } };

async function delegationCommand(command: string, args: string[], cwd?: string): Promise<string> {
  const result = await delegationExec(command, args, { cwd, timeout: 10000, maxBuffer: 1024 * 1024 });
  return result.stdout.trim();
}

async function delegationCanonicalPath(path: string): Promise<string> {
  try {
    return await realpath(path);
  } catch (error) {
    if (!(error instanceof Error) || !('code' in error) || error.code !== 'ENOENT') throw error;
    // A dangling symlink is not a new file: resolving its parent would conceal
    // its target and could authorize creating a file outside the worktree.
    try {
      if ((await lstat(path)).isSymbolicLink()) throw new Error('Dangling symlink path cannot be verified.');
    } catch (statError) {
      if (!(statError instanceof Error) || !('code' in statError) || statError.code !== 'ENOENT') throw statError;
    }
    const parent = delegationDirname(path);
    if (parent === path) throw error;
    return resolve(await delegationCanonicalPath(parent), relative(parent, path));
  }
}

function delegationContained(root: string, path: string): boolean {
  const suffix = relative(root, path);
  return suffix !== '' && suffix !== '..' && !suffix.startsWith('..' + sep) && !isAbsolute(suffix) && !suffix.split(sep).includes('.git');
}

// This is a trusted-host, single-plugin-runtime boundary, not an OS sandbox or a
// cross-process lock. Keep guards after completion so a resumed child cannot write.
export function registerLoafDelegation(amp: DelegationAPI, command: DelegationCommand = delegationCommand): {
  owns: (id: string) => boolean;
  check: (event: DelegationToolCall) => Promise<string | undefined>;
} {
  const children = new Map<string, DelegationGuard>();
  let writer = false;
  const check = async (event: DelegationToolCall): Promise<string | undefined> => {
    const guard = children.get(event.thread.id);
    if (!guard) return undefined;
    if (guard.sealed || guard.role === 'review') return 'Loaf child has no tool authority.';
    if (event.tool !== 'Read' && event.tool !== 'apply_patch') return 'Loaf implementation child allows only Read and apply_patch.';
    try {
      let paths: string[];
      if (event.tool === 'Read') {
        if (typeof event.input.path !== 'string' || !isAbsolute(event.input.path)) return 'Loaf requires an absolute Read path.';
        paths = [event.input.path];
      } else {
        // Amp's helper can omit relative paths and Move sources. This bounded
        // adapter accepts only explicit absolute patch headers and checks both.
        const patch = event.input.patchText;
        if (typeof patch !== 'string') return 'Loaf requires apply_patch.patchText with absolute file paths.';
        const headers = patch.split('\n').filter(line => line.startsWith('*** '));
        const declared: string[] = [];
        for (const header of headers) {
          if (['*** Begin Patch', '*** End Patch', '*** End of File'].includes(header)) continue;
          const match = /^\*\*\* (?:Add File|Update File|Delete File|Move to): (.+)$/.exec(header);
          if (!match || match[1] !== match[1].trim() || !isAbsolute(match[1])) return 'Loaf requires known patch headers and absolute file paths.';
          declared.push(match[1]);
        }
        if (!declared.length) return 'Loaf cannot verify every patch path.';
        const modified = amp.helpers.filesModifiedByToolCall(event);
        if (!modified?.length) return 'Loaf cannot verify every patch path.';
        paths = [...declared, ...modified.map(uri => amp.helpers.filePathFromURI(uri))];
      }
      for (const path of paths) {
        const absolute = resolve(guard.root, path);
        if (!delegationContained(guard.root, absolute) || !delegationContained(guard.root, await delegationCanonicalPath(absolute))) {
          return 'Loaf child path is outside its worktree or targets Git metadata.';
        }
      }
      return guard.sealed ? 'Loaf child has no tool authority.' : undefined;
    } catch {
      return 'Loaf could not verify the child tool boundary.';
    }
  };

  if (typeof amp.registerTool !== 'function') {
    console.warn('[loaf] Native delegation unavailable: installed Amp has no tool registration API.');
    return { owns: id => children.has(id), check };
  }
  const tool: PluginToolDefinition = {
    name: 'loaf_delegate',
    description: 'Delegate one native tracker contract in the current local Git worktree. Implementation has Read/apply_patch only with absolute paths; review has no tools and needs a complete immutable snapshot in packet. The main agent owns shell/tests, providers, acceptance, and exclusion of parent or unrelated writers. Inherits the current native Amp mode; no model selection. Prevents overlapping delegated implementers in this plugin runtime only; not an OS sandbox. Returns child identity, packet digest and turn evidence, never acceptance.',
    inputSchema: {
      type: 'object', additionalProperties: false,
      properties: {
        role: { type: 'string', enum: ['implementation', 'review'] },
        native_ref: { type: 'string', description: 'Canonical tracker record reference already read by the main agent.' },
        worktree: { type: 'string', description: 'Absolute path of the existing current Amp Git worktree.' },
        packet: { type: 'string', description: 'Live contract and bounded task; for review include exact immutable diff, sources and test evidence.' },
      },
      required: ['role', 'native_ref', 'worktree', 'packet'],
    },
    execute: async (input: Record<string, unknown>, ctx: PluginToolContext): Promise<string> => {
      let child: PluginThread | undefined;
      let guard: DelegationGuard | undefined;
      let acquired = false;
      let stopped = false;
      let parentStopped = false;
      let submissionPending = false;
      let parentSubscription: Subscription | undefined;
      let cancellation: Promise<void> | undefined;
      let interrupt: (error: Error) => void = () => {};
      const interrupted = new Promise<never>((_resolve, reject) => { interrupt = reject; });
      // Cancellation can arrive during preflight, before the promise is raced.
      void interrupted.catch(() => {});
      const cancelChild = (again = false): Promise<void> => {
        if (guard) guard.sealed = true;
        if ((!cancellation || again) && child) {
          try { cancellation = child.cancel().catch(() => {}); } catch { cancellation = Promise.resolve(); }
        }
        return cancellation ?? Promise.resolve();
      };
      const receipt: Record<string, unknown> = { acceptance: 'main-agent-required', parent_thread_id: ctx.thread.id };
      try {
        if (input.role !== 'implementation' && input.role !== 'review') throw new Error('role must be implementation or review');
        for (const field of ['native_ref', 'worktree', 'packet']) {
          if (typeof input[field] !== 'string' || !(input[field] as string).trim()) throw new Error(`${field} must be nonempty`);
        }
        const { role } = input;
        const worktree = input.worktree as string;
        const packet = input.packet as string;
        if (!isAbsolute(worktree)) throw new Error('worktree must be absolute');
        if (role === 'implementation') {
          if (writer) throw new Error('An implementation child is active or its stop is uncertain; inspect it before continuing.');
          writer = true;
          acquired = true;
        }
        if (typeof amp.createAgent !== 'function' || typeof ctx.thread.agent !== 'function' ||
            typeof ctx.thread.state?.subscribe !== 'function' || typeof ctx.thread.state?.get !== 'function' ||
            typeof amp.helpers?.filePathFromURI !== 'function' || typeof amp.helpers?.filesModifiedByToolCall !== 'function') {
          throw new Error('Installed Amp lacks required stable delegation APIs.');
        }
        const observeParent = (state: string): void => {
          if (state !== 'idle' && state !== 'error') return;
          parentStopped = true;
          void cancelChild();
          interrupt(new Error('Parent turn stopped; child work was interrupted.'));
        };
        parentSubscription = ctx.thread.state.subscribe(observeParent);
        observeParent(await ctx.thread.state.get());
        if (amp.system?.executor?.kind !== 'local' || !amp.system.workspaceRoot) throw new Error('Delegation requires the current local workspace executor.');
        const root = await realpath(worktree);
        const workspace = await realpath(amp.helpers.filePathFromURI(amp.system.workspaceRoot));
        if (root !== workspace || root !== await realpath(await command('git', ['rev-parse', '--show-toplevel'], root))) {
          throw new Error('Requested worktree must be the canonical current Amp Git worktree root.');
        }
        if (parentStopped) throw new Error('Parent turn stopped before child creation.');
        const agent = await ctx.thread.agent();
        const mode = agent.definition.kind === 'builtin-agent' ? agent.definition.mode : undefined;
        if (!mode || !['low', 'medium', 'high', 'ultra', 'smart', 'deep', 'rush'].includes(mode)) {
          throw new Error('Select a built-in Amp mode before delegation; custom model or effort overrides cannot be inherited safely.');
        }
        const tools = role === 'implementation' ? ['Read', 'apply_patch'] : [];
        if (tools.length) {
          const catalog: unknown = JSON.parse(await command('amp', ['plugins', 'show-agent-options', '--json']));
          const names = catalog && typeof catalog === 'object' && 'builtinToolNames' in catalog ? catalog.builtinToolNames : undefined;
          if (!Array.isArray(names) || names.some(name => typeof name !== 'string') || tools.some(tool => !names.includes(tool))) {
            throw new Error('Installed Amp does not expose the required minimal tool catalog.');
          }
        }
        if (parentStopped) throw new Error('Parent turn stopped before child creation.');
        Object.assign(receipt, { native_ref: input.native_ref, role, worktree: root, native_mode: mode, requested_tools: tools, packet_sha256: createHash('sha256').update(packet).digest('hex') });
        const childAgent = amp.createAgent({
          extends: mode, tools,
          instructions: role === 'review'
            ? 'Review only the supplied immutable snapshot. You have no tools. Cite precise source lines and missing evidence; do not approve work beyond that snapshot. The main agent decides acceptance.'
            : 'Implement only the supplied bounded native tracker contract in the current worktree. You have only Read and apply_patch. Use absolute paths for Read and every apply_patch file and Move header. Do not seek shell, provider, or delegation tools. Return changed paths and evidence needed from main-agent tests. The main agent owns tests, Git, tracker operations and acceptance.',
        });
        if (typeof childAgent.createThread !== 'function') throw new Error('Installed Amp cannot create a native child.');
        child = await childAgent.createThread({ parentThreadID: ctx.thread.id, executor: 'local', visibility: 'private' });
        receipt.child_thread_id = child.id;
        guard = { role, root, sealed: false };
        children.set(child.id, guard);
        if (parentStopped) throw new Error('Parent turn stopped during child creation.');
        if (typeof child.waitForResponse !== 'function' || typeof child.appendUserMessage !== 'function' || typeof child.cancel !== 'function' || typeof child.state?.get !== 'function') {
          throw new Error('Installed Amp lacks required child response or cancellation APIs.');
        }
        // Subscribe before append: waitForResponse observes the next running -> idle
        // transition. Capture rejection immediately while append is in flight.
        const response = child.waitForResponse({ timeoutMs: 600000 }).then(reply => ({ reply }), error => ({ error }));
        submissionPending = true;
        const submittingChild = child;
        const submission = Promise.resolve().then(() => {
          if (parentStopped) throw new Error('Parent turn stopped before child submission.');
          return submittingChild.appendUserMessage({ type: 'user-message', content: `Native record: ${input.native_ref}\nWorktree: ${root}\nPacket SHA-256: ${receipt.packet_sha256}\n\n${packet}` });
        }).finally(() => {
          submissionPending = false;
          // A previously idle child may start only after append settles. Cancel
          // again, but never silently release ownership after an uncertain result.
          if (parentStopped || guard?.sealed) void cancelChild(true);
        });
        await Promise.race([submission, interrupted]);
        const result = await Promise.race([response, interrupted]);
        if ('error' in result) throw result.error;
        if (parentStopped) throw new Error('Parent turn stopped before child acceptance.');
        stopped = true;
        const text = result.reply.content.filter(block => block.type === 'text').map(block => block.text).join('\n');
        return JSON.stringify({ ...receipt, status: 'turn-complete', signal: 'waitForResponse running-to-idle', text });
      } catch (error) {
        let state: string | undefined;
        const unresolvedSubmission = submissionPending;
        if (child) {
          await cancelChild();
          try { state = await child.state.get(); stopped = !unresolvedSubmission && !submissionPending && (state === 'idle' || state === 'error'); } catch { stopped = false; }
        }
        return JSON.stringify({ ...receipt, status: child ? (stopped ? 'interrupted' : 'uncertain') : (parentStopped ? 'interrupted' : 'incompatible'), child_state: state,
          submission_pending: submissionPending,
          message: error instanceof Error ? error.message : 'Delegation failed; inspect child state.' });
      } finally {
        parentSubscription?.unsubscribe();
        if (guard) guard.sealed = true;
        if (acquired && (!child || stopped)) writer = false;
      }
    },
  };
  try {
    amp.registerTool(tool);
  } catch (error) {
    console.warn('[loaf] Native delegation unavailable; existing hooks remain active:', error instanceof Error ? error.message : 'tool registration failed');
  }
  return { owns: id => children.has(id), check };
}
