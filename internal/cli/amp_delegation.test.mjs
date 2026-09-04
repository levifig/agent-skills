import assert from 'node:assert/strict';
import { mkdtemp, mkdir, realpath, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import { registerLoafDelegation } from './amp_delegation.ts';

test('generated plugin preserves normal hook dispatch when optional delegation registration fails', async t => {
  const { default: initialize } = await import('../../dist/amp/.amp/plugins/loaf.ts');
  t.mock.method(console, 'warn', () => {});
  const handlers = new Map();
  const amp = {
    registerTool() { throw new Error('duplicate registration'); },
    on(event, handler) { handlers.set(event, handler); },
    helpers: { shellCommandFromToolCall: () => null },
  };
  assert.doesNotThrow(() => initialize(amp));
  assert.deepEqual([...handlers.keys()], ['agent.start', 'tool.call', 'tool.result']);
  const result = await handlers.get('tool.call')({ toolUseID: 'probe', tool: 'unmatched_probe', input: {}, thread: { id: 'T-parent' } });
  assert.deepEqual(result, { action: 'allow' });
  await handlers.get('tool.result')({ toolUseID: 'probe', tool: 'unmatched_probe', input: {}, thread: { id: 'T-parent' }, status: 'done' });
});

async function fixture(t, overrides = {}) {
  const root = await realpath(await mkdtemp(join(tmpdir(), 'loaf-amp-')));
  t.after(() => rm(root, { recursive: true, force: true }));
  let tool;
  let parentObserver;
  const calls = [];
  const child = {
    id: 'T-child',
    waitForResponse: async () => ({ role: 'assistant', content: [{ type: 'text', text: 'Evidence' }] }),
    appendUserMessage: async message => calls.push(['append', message]),
    cancel: async () => calls.push(['cancel']),
    state: { get: async () => 'idle' },
    ...overrides.child,
  };
  const amp = {
    system: { workspaceRoot: pathToFileURL(root), executor: { kind: 'local' } },
    helpers: {
      filePathFromURI: uri => new URL(uri.toString()).pathname,
      filesModifiedByToolCall: event => event.input.paths?.map(path => pathToFileURL(path)) ?? null,
    },
    registerTool: definition => { tool = definition; },
    createAgent: config => { calls.push(['agent', config]); return { createThread: async options => { calls.push(['thread', options]); return child; } }; },
    ...overrides.amp,
  };
  const guard = registerLoafDelegation(amp, overrides.command ?? (async (command, args, cwd) => {
    calls.push(['command', command, args, cwd]);
    return command === 'git' ? root : JSON.stringify({ builtinToolNames: ['Read', 'apply_patch'] });
  }));
  const ctx = { thread: {
    id: 'T-parent', agent: async () => ({ definition: { kind: 'builtin-agent', mode: 'high' } }),
    state: { get: async () => 'running', subscribe: observer => { parentObserver = observer; return { unsubscribe: () => { calls.push(['unsubscribe']); parentObserver = undefined; } }; } },
  }, ...overrides.ctx };
  const input = { role: 'implementation', native_ref: 'https://github.com/example/repo/issues/1', worktree: root, packet: 'Contract and task' };
  return { root, tool, guard, calls, child, ctx, input, stopParent: state => parentObserver?.(state) };
}

test('native child inherits mode, has minimal tools, and returns provenance, snapshot hash and turn evidence', async t => {
  const f = await fixture(t);
  const result = JSON.parse(await f.tool.execute(f.input, f.ctx));
  assert.equal(result.status, 'turn-complete');
  assert.equal(result.parent_thread_id, 'T-parent');
  assert.equal(result.child_thread_id, 'T-child');
  assert.match(result.packet_sha256, /^[a-f0-9]{64}$/);
  assert.equal(result.acceptance, 'main-agent-required');
  assert.deepEqual(f.calls.find(([name]) => name === 'agent')[1].tools, ['Read', 'apply_patch']);
  assert.equal(f.calls.find(([name]) => name === 'agent')[1].extends, 'high');
  assert.equal(f.calls.find(([name]) => name === 'agent')[1].model, undefined);
  assert.deepEqual(f.calls.find(([name]) => name === 'thread')[1], { parentThreadID: 'T-parent', executor: 'local', visibility: 'private' });
  assert.ok(await f.guard.check({ thread: { id: 'T-child' }, tool: 'Read', input: { path: join(f.root, 'source') } }));
});

test('reviewer receives immutable snapshot and no tools', async t => {
  const f = await fixture(t);
  await f.tool.execute({ ...f.input, role: 'review', packet: 'diff + exact source' }, f.ctx);
  assert.deepEqual(f.calls.find(([name]) => name === 'agent')[1].tools, []);
  assert.match(f.calls.find(([name]) => name === 'append')[1].content, /diff \+ exact source/);
});

test('review does not require the implementation tool catalog', async t => {
  const f = await fixture(t, { command: async (command, _args, cwd) => {
    if (command === 'git') return cwd;
    throw new Error('catalog unavailable');
  } });
  const result = JSON.parse(await f.tool.execute({ ...f.input, role: 'review' }, f.ctx));
  assert.equal(result.status, 'turn-complete');
  assert.deepEqual(result.requested_tools, []);
});

test('parent cancellation during preflight is interrupted rather than incompatible', async t => {
  let release;
  let started;
  const commandStarted = new Promise(resolve => { started = resolve; });
  const f = await fixture(t, { command: async (_command, _args, cwd) => {
    started();
    await new Promise(resolve => { release = resolve; });
    return cwd;
  } });
  const pending = f.tool.execute(f.input, f.ctx);
  await commandStarted;
  f.stopParent('idle');
  release();
  const result = JSON.parse(await pending);
  assert.equal(result.status, 'interrupted');
  assert.equal(result.child_thread_id, undefined);
  assert.equal(f.calls.some(([name]) => name === 'agent'), false);
});

test('invalid input and unsupported parent do not spawn', async t => {
  const f = await fixture(t);
  for (const input of [{ ...f.input, role: 'other' }, { ...f.input, packet: '' }, { ...f.input, worktree: '/' }]) {
    assert.equal(JSON.parse(await f.tool.execute(input, f.ctx)).status, 'incompatible');
  }
  const unsupported = await fixture(t, { ctx: { thread: { id: 'T-parent', agent: async () => ({ definition: { kind: 'agent-definition' } }) } } });
  assert.equal(JSON.parse(await unsupported.tool.execute(unsupported.input, unsupported.ctx)).status, 'incompatible');
  assert.equal(f.calls.some(([name]) => name === 'agent'), false);
});

test('one writer and active guard reject shell, providers, unknown paths, symlink escapes, moves and Git metadata', async t => {
  let finish;
  let appended;
  const started = new Promise(resolve => { appended = resolve; });
  const f = await fixture(t, { child: {
    waitForResponse: () => new Promise(resolve => { finish = resolve; }),
    appendUserMessage: async () => appended(),
  } });
  const pending = f.tool.execute(f.input, f.ctx);
  await started;
  assert.equal(JSON.parse(await f.tool.execute(f.input, f.ctx)).status, 'incompatible');
  await symlink(tmpdir(), join(f.root, 'escape'));
  await symlink(join(tmpdir(), 'loaf-missing-target'), join(f.root, 'dangling'));
  await mkdir(join(f.root, '.git'));
  for (const event of [
    { tool: 'Bash', input: { command: 'echo bad' } },
    { tool: 'mcp_linear_create', input: {} },
    { tool: 'apply_patch', input: {} },
    { tool: 'Read', input: { path: join(f.root, 'escape', 'outside') } },
    { tool: 'Read', input: { path: join(f.root, '.git', 'config') } },
    { tool: 'Read', input: { path: join(f.root, 'dangling') } },
    { tool: 'Read', input: { path: 'relative.txt' } },
    { tool: 'Read', input: { path: pathToFileURL(join(f.root, 'okay')).toString() } },
    { tool: 'apply_patch', input: { paths: [join(f.root, 'okay'), '/outside'] } },
    { tool: 'apply_patch', input: { patchText: `*** Begin Patch\n*** Update File: /outside\n*** Move to: ${join(f.root, 'okay')}\n*** End Patch`, paths: [join(f.root, 'okay')] } },
    { tool: 'apply_patch', input: { patchText: `*** Begin Patch\n*** Update File: relative.txt\n*** Move to: ${join(f.root, 'okay')}\n*** End Patch`, paths: [join(f.root, 'okay')] } },
    { tool: 'apply_patch', input: { patchText: `*** Begin Patch\n*** Unknown: ${join(f.root, 'okay')}\n*** End Patch`, paths: [join(f.root, 'okay')] } },
  ]) assert.ok(await f.guard.check({ ...event, thread: { id: 'T-child' } }), JSON.stringify(event));
  await writeFile(join(f.root, 'okay'), 'source');
  assert.equal(await f.guard.check({ tool: 'Read', input: { path: join(f.root, 'okay') }, thread: { id: 'T-child' } }), undefined);
  assert.equal(await f.guard.check({ tool: 'apply_patch', input: { patchText: `*** Begin Patch\n*** Update File: ${join(f.root, 'okay')}\n@@\n-source\n+updated\n*** End Patch`, paths: [join(f.root, 'okay')] }, thread: { id: 'T-child' } }), undefined);
  assert.equal(await f.guard.check({ tool: 'other', input: {}, thread: { id: 'T-unrelated' } }), undefined);
  finish({ role: 'assistant', content: [{ type: 'text', text: 'done' }] });
  await pending;
});

test('wait failure cancels, reports uncertain state and retains writer lock', async t => {
  const f = await fixture(t, { child: { waitForResponse: async () => { throw new Error('timed out'); }, state: { get: async () => 'running' } } });
  const result = JSON.parse(await f.tool.execute(f.input, f.ctx));
  assert.equal(result.status, 'uncertain');
  assert.equal(result.child_thread_id, 'T-child');
  assert.ok(f.calls.some(([name]) => name === 'cancel'));
  assert.equal(JSON.parse(await f.tool.execute(f.input, f.ctx)).status, 'incompatible');
});

test('custom model routing, missing API, missing required tools and remote executors fail closed', async t => {
  for (const overrides of [
    { ctx: { thread: { id: 'T-parent', agent: async () => ({ definition: { kind: 'agent-definition', extends: 'high', model: 'operator/choice' } }) } } },
    { amp: { createAgent: undefined } },
    { amp: { system: { executor: { kind: 'orb' } } } },
    { command: async (command, args, cwd) => command === 'git' ? cwd : JSON.stringify({ builtinToolNames: ['Read'] }) },
    { command: async (command, args, cwd) => command === 'git' ? cwd : 'malformed' },
  ]) {
    const f = await fixture(t, overrides);
    assert.equal(JSON.parse(await f.tool.execute(f.input, f.ctx)).status, 'incompatible');
    assert.equal(f.calls.some(([name]) => name === 'agent'), false);
  }
});

test('waiter is established before submission, and append failure cannot escape guard sealing', async t => {
  let waiting = false;
  const f = await fixture(t, { child: {
    waitForResponse: async () => { waiting = true; throw new Error('wait failed'); },
    appendUserMessage: async () => { assert.equal(waiting, true); throw new Error('append failed'); },
  } });
  const result = JSON.parse(await f.tool.execute(f.input, f.ctx));
  assert.equal(result.status, 'interrupted');
  assert.match(result.message, /append failed/);
  assert.ok(await f.guard.check({ thread: { id: 'T-child' }, tool: 'Read', input: { path: join(f.root, 'file') } }));
});

test('review guard rejects every tool during the turn', async t => {
  let finish;
  let appended;
  const started = new Promise(resolve => { appended = resolve; });
  const f = await fixture(t, { child: {
    waitForResponse: () => new Promise(resolve => { finish = resolve; }),
    appendUserMessage: async () => appended(),
  } });
  const pending = f.tool.execute({ ...f.input, role: 'review' }, f.ctx);
  await started;
  for (const tool of ['Read', 'apply_patch', 'Bash', 'mcp_linear_create', 'loaf_delegate']) {
    assert.ok(await f.guard.check({ tool, input: { path: join(f.root, 'file') }, thread: { id: 'T-child' } }));
  }
  finish({ role: 'assistant', content: [{ type: 'text', text: 'reviewed supplied evidence' }] });
  await pending;
});

test('parent cancellation during creation cancels child without appending work', async t => {
  let created;
  let creating;
  const creationStarted = new Promise(resolve => { creating = resolve; });
  const f = await fixture(t, { amp: { createAgent: () => ({ createThread: () => { creating(); return new Promise(resolve => { created = resolve; }); } }) } });
  const pending = f.tool.execute(f.input, f.ctx);
  await creationStarted;
  f.stopParent('idle');
  created(f.child);
  const result = JSON.parse(await pending);
  assert.equal(result.status, 'interrupted');
  assert.equal(f.calls.some(([name]) => name === 'append'), false);
  assert.ok(f.calls.some(([name]) => name === 'cancel'));
  assert.ok(f.calls.some(([name]) => name === 'unsubscribe'));
});

test('parent cancellation seals running child and cannot become turn-complete', async t => {
  let finish;
  let appended;
  const started = new Promise(resolve => { appended = resolve; });
  const f = await fixture(t, { child: {
    waitForResponse: () => new Promise(resolve => { finish = resolve; }),
    appendUserMessage: async () => appended(),
  } });
  const pending = f.tool.execute(f.input, f.ctx);
  await started;
  await new Promise(resolve => setImmediate(resolve));
  f.stopParent('idle');
  assert.ok(await f.guard.check({ thread: { id: 'T-child' }, tool: 'Read', input: { path: join(f.root, 'file') } }));
  finish({ role: 'assistant', content: [{ type: 'text', text: 'late reply' }] });
  assert.equal(JSON.parse(await pending).status, 'interrupted');
  assert.ok(f.calls.some(([name]) => name === 'cancel'));
});

test('failed parent-triggered cancellation retains uncertain writer lock', async t => {
  let appended;
  const started = new Promise(resolve => { appended = resolve; });
  const f = await fixture(t, { child: {
    waitForResponse: () => new Promise(() => {}),
    appendUserMessage: async () => appended(),
    cancel: async () => { throw new Error('cancel unavailable'); },
    state: { get: async () => 'running' },
  } });
  const pending = f.tool.execute(f.input, f.ctx);
  await started;
  f.stopParent('error');
  assert.equal(JSON.parse(await pending).status, 'uncertain');
  assert.equal(JSON.parse(await f.tool.execute(f.input, f.ctx)).status, 'incompatible');
});

test('pending submission remains owned and is cancelled again when it settles after parent stop', async t => {
  let settleSubmission;
  let submitting;
  let state = 'idle';
  let cancelCount = 0;
  const submissionStarted = new Promise(resolve => { submitting = resolve; });
  const f = await fixture(t, { child: {
    waitForResponse: () => new Promise(() => {}),
    appendUserMessage: () => { submitting(); return new Promise(resolve => { settleSubmission = () => { state = 'running'; resolve(); }; }); },
    cancel: async () => { cancelCount++; state = 'idle'; },
    state: { get: async () => state },
  } });
  const pending = f.tool.execute(f.input, f.ctx);
  await submissionStarted;
  f.stopParent('idle');
  const result = JSON.parse(await pending);
  assert.equal(result.status, 'uncertain');
  assert.equal(result.submission_pending, true);
  assert.equal(JSON.parse(await f.tool.execute(f.input, f.ctx)).status, 'incompatible');
  settleSubmission();
  await new Promise(resolve => setImmediate(resolve));
  assert.ok(cancelCount >= 2);
  assert.equal(state, 'idle');
  assert.equal(JSON.parse(await f.tool.execute(f.input, f.ctx)).status, 'incompatible');
});
