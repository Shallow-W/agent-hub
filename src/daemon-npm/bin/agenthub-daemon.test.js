const assert = require('node:assert/strict');
const test = require('node:test');
const { EventEmitter } = require('node:events');
const { onWebSocket } = require('./agenthub-daemon.js');

test('onWebSocket supports ws EventEmitter clients', () => {
  const ws = new EventEmitter();
  let called = false;

  onWebSocket(ws, 'open', () => {
    called = true;
  });
  ws.emit('open');

  assert.equal(called, true);
});

test('onWebSocket adapts WHATWG message events', () => {
  const listeners = new Map();
  const ws = {
    addEventListener(name, handler) {
      listeners.set(name, handler);
    },
  };
  let message = '';

  onWebSocket(ws, 'message', (data) => {
    message = data;
  });
  listeners.get('message')({ data: '{"type":"ping"}' });

  assert.equal(message, '{"type":"ping"}');
});

test('onWebSocket adapts WHATWG close events', () => {
  const listeners = new Map();
  const ws = {
    addEventListener(name, handler) {
      listeners.set(name, handler);
    },
  };
  let closeCode = 0;
  let closeReason = '';

  onWebSocket(ws, 'close', (code, reason) => {
    closeCode = code;
    closeReason = reason;
  });
  listeners.get('close')({ code: 1006, reason: 'network' });

  assert.equal(closeCode, 1006);
  assert.equal(closeReason, 'network');
});
