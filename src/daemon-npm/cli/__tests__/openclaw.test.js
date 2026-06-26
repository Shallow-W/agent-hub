'use strict';

const { test } = require('node:test');
const assert = require('node:assert');

const { createOpenClawCliSpec } = require('../openclaw');

function buildMockCtx() {
  return {
    defaultSkills: (caps) => caps.map((id) => ({ id, name: id })),
    resolveCommand: (name) => `/bin/${name}`,
    commandVersion: () => '1.0.0',
    processSpec: (command, args) => ({ command, args }),
    spawnSync: () => ({ status: 0, stdout: '', stderr: '' }),
    firstLine: (s) => String(s || '').split('\n')[0],
    logFlow: () => {},
    pathJoin: (...args) => args.join('/'),
    addRoot: (arr, root) => { if (arr.indexOf(root) === -1) arr.push(root); },
    isAgentHubWorkspace: () => false,
    openClawInstallSkillRoots: () => [],
  };
}

test('openclaw.resolveCommand returns literal "openclaw"', () => {
  const spec = createOpenClawCliSpec(buildMockCtx());
  assert.strictEqual(spec.resolveCommand(), 'openclaw');
});

test('openclaw.parseResult returns fallback for empty stdout', () => {
  const spec = createOpenClawCliSpec(buildMockCtx());
  assert.strictEqual(spec.parseResult({ stdout: '' }), '(OpenClaw CLI 没有返回内容)');
  assert.strictEqual(spec.parseResult({}), '(OpenClaw CLI 没有返回内容)');
});

test('openclaw.parseResult prefers finalAssistantVisibleText', () => {
  const spec = createOpenClawCliSpec(buildMockCtx());
  const stdout = JSON.stringify({
    finalAssistantVisibleText: 'visible',
    finalAssistantRawText: 'raw',
  });
  assert.strictEqual(spec.parseResult({ stdout }), 'visible');
});

test('openclaw.parseResult falls back to finalAssistantRawText', () => {
  const spec = createOpenClawCliSpec(buildMockCtx());
  const stdout = JSON.stringify({ finalAssistantRawText: 'raw only' });
  assert.strictEqual(spec.parseResult({ stdout }), 'raw only');
});

test('openclaw.parseResult extracts payloads array', () => {
  const spec = createOpenClawCliSpec(buildMockCtx());
  const stdout = JSON.stringify({
    payloads: [{ text: 'first' }, { text: 'second' }],
  });
  assert.strictEqual(spec.parseResult({ stdout }), 'first\nsecond');
});

test('openclaw.parseResult extracts assistant messages', () => {
  const spec = createOpenClawCliSpec(buildMockCtx());
  const stdout = JSON.stringify({
    messages: [
      { role: 'user', content: 'q' },
      { role: 'assistant', content: 'a1' },
      { role: 'assistant', content: 'a2' },
    ],
  });
  assert.strictEqual(spec.parseResult({ stdout }), 'a1\na2');
});

test('openclaw.parseResult extracts content field', () => {
  const spec = createOpenClawCliSpec(buildMockCtx());
  const stdout = JSON.stringify({ content: 'plain content' });
  assert.strictEqual(spec.parseResult({ stdout }), 'plain content');
});

test('openclaw.parseResult tries response/result/output/text/message fields in order', () => {
  const spec = createOpenClawCliSpec(buildMockCtx());
  assert.strictEqual(spec.parseResult({ stdout: JSON.stringify({ response: 'resp' }) }), 'resp');
  assert.strictEqual(spec.parseResult({ stdout: JSON.stringify({ result: 'res' }) }), 'res');
  assert.strictEqual(spec.parseResult({ stdout: JSON.stringify({ output: 'out' }) }), 'out');
});

test('openclaw.parseResult extracts nested data.text', () => {
  const spec = createOpenClawCliSpec(buildMockCtx());
  const stdout = JSON.stringify({ data: { text: 'nested' } });
  assert.strictEqual(spec.parseResult({ stdout }), 'nested');
});

test('openclaw.parseResult returns raw stdout on JSON parse failure', () => {
  const spec = createOpenClawCliSpec(buildMockCtx());
  assert.strictEqual(spec.parseResult({ stdout: 'plain text not json' }), 'plain text not json');
});

test('openclaw.parseResult returns raw stdout when JSON has no recognizable field', () => {
  const spec = createOpenClawCliSpec(buildMockCtx());
  const stdout = JSON.stringify({ unrelated: 'x' });
  // Falls through all branches, returns original text
  assert.strictEqual(spec.parseResult({ stdout }), stdout);
});
