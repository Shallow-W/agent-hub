import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import {
  backendReadyURL,
  resolveBackendBaseURL,
  resolvePreloadConfig,
  resolveFrontendURL,
  waitForHTTP,
} from './runtime.mjs';

describe('Electron runtime helpers', () => {
  it('uses Vite URL and does not launch backend in development', () => {
    const env = {
      VITE_DEV_SERVER_URL: 'http://127.0.0.1:5173',
      AGENTHUB_BACKEND_URL: 'http://127.0.0.1:8080',
    };

    assert.equal(resolveFrontendURL({ env, appPath: '/app', resourcesPath: '/resources' }), 'http://127.0.0.1:5173');
  });

  it('loads the packaged frontend and does not launch a local backend in production', () => {
    const env = {};
    const resourcesPath = path.join('C:', 'AgentHub', 'resources');

    assert.equal(
      resolveFrontendURL({ env, appPath: '/app', resourcesPath }),
      pathToFileURL(path.join(resourcesPath, 'frontend-dist', 'index.html')).href,
    );
  });

  it('uses explicit backend URL as the desktop API base without launching backend', () => {
    const env = {
      AGENTHUB_BACKEND_URL: 'http://10.0.0.5:8080',
    };

    assert.equal(resolveBackendBaseURL(env), 'http://10.0.0.5:8080');
    assert.equal(
      resolveFrontendURL({ env, appPath: '/app', resourcesPath: '/resources' }),
      pathToFileURL(path.join('/resources', 'frontend-dist', 'index.html')).href,
    );
  });

  it('uses the remote backend by default', () => {
    assert.equal(resolveBackendBaseURL({}), 'http://10.11.221.79:8080');
  });

  it('passes the backend API base to the preload bridge', () => {
    const env = {
      AGENTHUB_BACKEND_URL: 'https://agenthub.example.com/',
    };

    assert.deepEqual(resolvePreloadConfig(env), {
      backendBaseURL: 'https://agenthub.example.com',
    });
  });

  it('uses the remote backend ready endpoint for diagnostics', () => {
    assert.equal(backendReadyURL(), 'http://10.11.221.79:8080/health/ready');
  });

  it('waits until an HTTP endpoint becomes available', async () => {
    let attempts = 0;

    await waitForHTTP('http://127.0.0.1:8080/health', {
      timeoutMs: 1000,
      intervalMs: 1,
      fetchImpl: async () => {
        attempts += 1;
        return { ok: attempts === 3 };
      },
    });

    assert.equal(attempts, 3);
  });

  it('fails when an HTTP endpoint never becomes available', async () => {
    await assert.rejects(
      () => waitForHTTP('http://127.0.0.1:8080/health', {
        timeoutMs: 5,
        intervalMs: 1,
        fetchImpl: async () => ({ ok: false }),
      }),
      /Timed out waiting for/,
    );
  });
});
