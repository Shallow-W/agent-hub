import path from 'node:path';
import { pathToFileURL } from 'node:url';

const DEFAULT_BACKEND_URL = 'http://10.11.221.79:8080';

function normalizeBaseURL(value) {
  return value ? value.replace(/\/+$/, '') : '';
}

export function resolveFrontendURL({ env, appPath, resourcesPath }) {
  if (env.VITE_DEV_SERVER_URL) {
    return env.VITE_DEV_SERVER_URL;
  }
  if (resourcesPath) {
    return pathToFileURL(resolveFrontendDistIndex({ resourcesPath })).href;
  }
  return pathToFileURL(path.join(appPath, 'dist', 'index.html')).href;
}

export function resolveBackendBaseURL(env) {
  return normalizeBaseURL(env.AGENTHUB_BACKEND_URL || DEFAULT_BACKEND_URL);
}

export function resolvePreloadConfig(env) {
  return {
    backendBaseURL: resolveBackendBaseURL(env),
  };
}

export function resolveFrontendDist({ resourcesPath }) {
  return path.join(resourcesPath, 'frontend-dist');
}

export function resolveFrontendDistIndex({ resourcesPath }) {
  return path.join(resolveFrontendDist({ resourcesPath }), 'index.html');
}

export function backendReadyURL() {
  return `${resolveBackendBaseURL({})}/health/ready`;
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
