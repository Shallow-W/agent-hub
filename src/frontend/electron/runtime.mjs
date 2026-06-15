import path from 'node:path';
import { existsSync } from 'node:fs';

const DEFAULT_BACKEND_URL = 'http://127.0.0.1:8080';

export function resolveFrontendURL({ env, appPath, resourcesPath }) {
  if (env.VITE_DEV_SERVER_URL) {
    return env.VITE_DEV_SERVER_URL;
  }
  if (env.AGENTHUB_BACKEND_URL) {
    return env.AGENTHUB_BACKEND_URL;
  }
  if (appPath || resourcesPath) {
    return DEFAULT_BACKEND_URL;
  }
  return DEFAULT_BACKEND_URL;
}

export function shouldLaunchBackend(env) {
  if (env.VITE_DEV_SERVER_URL) {
    return false;
  }
  if (env.AGENTHUB_DESKTOP_LAUNCH_BACKEND === 'false') {
    return false;
  }
  return !env.AGENTHUB_BACKEND_URL;
}

export function resolveBackendBinary({ resourcesPath, platform, exists = existsSync }) {
  const names = platform === 'win32' ? ['server.exe', 'server'] : ['server'];
  for (const name of names) {
    const candidate = path.join(resourcesPath, 'bin', name);
    if (exists(candidate)) {
      return candidate;
    }
  }
  return path.join(resourcesPath, 'bin', names[0]);
}

export function resolveConfigPath({ resourcesPath }) {
  return path.join(resourcesPath, 'config', 'config.yaml');
}

export function resolveFrontendDist({ resourcesPath }) {
  return path.join(resourcesPath, 'frontend-dist');
}

export function buildBackendEnv({ baseEnv, configPath, frontendDist }) {
  return {
    ...baseEnv,
    AGENTHUB_CONFIG: baseEnv.AGENTHUB_CONFIG || configPath,
    AGENTHUB_FRONTEND_DIST: frontendDist,
  };
}

export async function waitForHTTP(url, {
  timeoutMs = 15000,
  intervalMs = 250,
  fetchImpl = fetch,
} = {}) {
  const deadline = Date.now() + timeoutMs;
  let lastError;

  while (Date.now() < deadline) {
    try {
      const response = await fetchImpl(url);
      if (response.ok) {
        return;
      }
    } catch (error) {
      lastError = error;
    }
    await new Promise((resolve) => setTimeout(resolve, intervalMs));
  }

  const detail = lastError instanceof Error ? `: ${lastError.message}` : '';
  throw new Error(`Timed out waiting for ${url}${detail}`);
}
