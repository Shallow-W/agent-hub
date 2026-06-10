import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadConfigFromFile } from 'vite';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const projectRoot = path.resolve(__dirname, '..');
const configFile = path.join(projectRoot, 'vite.config.ts');

async function loadViteConfig(env = {}) {
  const previous = {};
  for (const [key, value] of Object.entries(env)) {
    previous[key] = process.env[key];
    process.env[key] = value;
  }

  try {
    const loaded = await loadConfigFromFile(
      { command: 'serve', mode: 'development' },
      configFile,
      projectRoot,
    );

    assert.ok(loaded);
    return loaded.config;
  } finally {
    for (const key of Object.keys(env)) {
      if (previous[key] === undefined) {
        delete process.env[key];
      } else {
        process.env[key] = previous[key];
      }
    }
  }
}

describe('web startup proxy config', () => {
  it('uses the remote backend by default for API and WebSocket proxying', async () => {
    const config = await loadViteConfig();

    assert.equal(config.server?.proxy?.['/api']?.target, 'http://10.11.221.79:8080');
    assert.equal(config.server?.proxy?.['/ws']?.target, 'ws://10.11.221.79:8080');
  });

  it('allows the web backend proxy target to be overridden by environment', async () => {
    const config = await loadViteConfig({
      VITE_AGENTHUB_BACKEND_URL: 'https://agenthub.example.com/',
    });

    assert.equal(config.server?.proxy?.['/api']?.target, 'https://agenthub.example.com');
    assert.equal(config.server?.proxy?.['/ws']?.target, 'wss://agenthub.example.com');
  });
});
