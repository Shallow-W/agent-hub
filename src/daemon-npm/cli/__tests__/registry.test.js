'use strict';

const { test } = require('node:test');
const assert = require('node:assert');

const {
  registerCliTool,
  getCliTool,
  allCliTools,
  clearCliTools,
} = require('../registry');

function makeSpec(cliTool) {
  return { cliTool, name: cliTool.toUpperCase(), buildCommand() { return {}; } };
}

test('registerCliTool requires spec.cliTool', () => {
  clearCliTools();
  assert.throws(() => registerCliTool(null), /cliTool/);
  assert.throws(() => registerCliTool({}), /cliTool/);
});

test('register / get / all roundtrip', () => {
  clearCliTools();
  registerCliTool(makeSpec('alpha'));
  registerCliTool(makeSpec('beta'));
  assert.ok(getCliTool('alpha'));
  assert.ok(getCliTool('beta'));
  assert.strictEqual(getCliTool('missing'), undefined);
  const all = allCliTools();
  assert.strictEqual(all.length, 2);
  const names = all.map((s) => s.cliTool).sort();
  assert.deepStrictEqual(names, ['alpha', 'beta']);
});

test('register is idempotent — last write wins', () => {
  clearCliTools();
  registerCliTool({ cliTool: 'x', name: 'first' });
  registerCliTool({ cliTool: 'x', name: 'second' });
  assert.strictEqual(getCliTool('x').name, 'second');
  assert.strictEqual(allCliTools().length, 1);
});

test('clearCliTools empties the registry', () => {
  registerCliTool(makeSpec('temp'));
  clearCliTools();
  assert.strictEqual(allCliTools().length, 0);
  assert.strictEqual(getCliTool('temp'), undefined);
});
