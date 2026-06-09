import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import {
  buildBackendEnv,
  backendReadyURL,
  resolveBackendBinary,
  resolveConfigPath,
  resolveFrontendURL,
  waitForHTTP,
  shouldLaunchBackend,
} from './runtime.mjs';

describe('Electron runtime helpers', () => {
  it('uses Vite URL and does not launch backend in development', () => {
    const env = {
      VITE_DEV_SERVER_URL: 'http://127.0.0.1:5173',
      AGENTHUB_BACKEND_URL: 'http://127.0.0.1:8080',
    };

    assert.equal(resolveFrontendURL({ env, appPath: '/app', resourcesPath: '/resources' }), 'http://127.0.0.1:5173');
    assert.equal(shouldLaunchBackend(env), false);
  });

  it('loads the bundled backend origin in production by default', () => {
    const env = {};

    assert.equal(resolveFrontendURL({ env, appPath: '/app', resourcesPath: '/resources' }), 'http://127.0.0.1:8080');
    assert.equal(shouldLaunchBackend(env), true);
  });

  it('honors explicit backend URL and disables bundled backend launch', () => {
    const env = {
      AGENTHUB_BACKEND_URL: 'http://10.0.0.5:8080',
      AGENTHUB_DESKTOP_LAUNCH_BACKEND: 'false',
    };

    assert.equal(resolveFrontendURL({ env, appPath: '/app', resourcesPath: '/resources' }), 'http://10.0.0.5:8080');
    assert.equal(shouldLaunchBackend(env), false);
  });

  it('resolves packaged resources for backend binary and config', () => {
    const resourcesPath = path.join('C:', 'AgentHub', 'resources');

    assert.equal(
      resolveBackendBinary({
        resourcesPath,
        platform: 'win32',
        exists: (candidate) => candidate.endsWith('server.exe'),
      }),
      path.join(resourcesPath, 'bin', 'server.exe'),
    );
    assert.equal(resolveConfigPath({ resourcesPath }), path.join(resourcesPath, 'config', 'config.yaml'));
  });

  it('falls back to extensionless backend binary for current build output', () => {
    const resourcesPath = path.join('C:', 'AgentHub', 'resources');

    assert.equal(
      resolveBackendBinary({
        resourcesPath,
        platform: 'win32',
        exists: (candidate) => candidate.endsWith('server'),
      }),
      path.join(resourcesPath, 'bin', 'server'),
    );
  });

  it('builds backend environment without clobbering existing variables', () => {
    const env = buildBackendEnv({
      baseEnv: { PATH: 'x', AGENTHUB_CONFIG: 'custom.yaml' },
      configPath: 'default.yaml',
      frontendDist: 'dist',
    });

    assert.equal(env.PATH, 'x');
    assert.equal(env.AGENTHUB_CONFIG, 'custom.yaml');
    assert.equal(env.AGENTHUB_FRONTEND_DIST, 'dist');
  });

  it('waits for the backend ready endpoint before opening the packaged desktop window', () => {
    assert.equal(backendReadyURL(), 'http://127.0.0.1:8080/health/ready');
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
