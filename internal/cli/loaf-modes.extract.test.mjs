import assert from 'node:assert/strict';
import test from 'node:test';
import { extractPinnedAgentText } from '../../content/amp/plugins/loaf-modes.ts';

test('extractPinnedAgentText keeps string replies', () => {
  assert.equal(extractPinnedAgentText('Implementation complete.'), 'Implementation complete.');
});

test('extractPinnedAgentText joins Amp text blocks and skips non-text blocks', () => {
  assert.equal(
    extractPinnedAgentText([
      { type: 'thinking', thinking: 'ignore this' },
      { type: 'text', text: 'First finding.' },
      { type: 'tool_use', id: 'tool-1', name: 'Read', input: { path: 'internal/cli/build.go' } },
      { type: 'text', text: 'Second finding.' },
      { type: 'text', text: 12 },
      null,
    ]),
    'First finding.\nSecond finding.',
  );
});

test('extractPinnedAgentText returns empty text for missing or empty replies', () => {
  assert.equal(extractPinnedAgentText(undefined), '');
  assert.equal(extractPinnedAgentText({ type: 'text', text: 'not an array' }), '');
  assert.equal(extractPinnedAgentText([]), '');
  assert.equal(
    extractPinnedAgentText([{ type: 'thinking', thinking: 'no visible text' }]),
    '',
  );
});
