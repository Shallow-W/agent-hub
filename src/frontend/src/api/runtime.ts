function normalizeBaseURL(value?: string): string {
  return value ? value.replace(/\/+$/, '') : '';
}

function desktopBackendBaseURL(): string {
  return normalizeBaseURL(window.agentHubDesktop?.backendBaseURL);
}

export function apiBaseURL(): string {
  return desktopBackendBaseURL();
}

export function apiURL(path: string): string {
  if (/^https?:\/\//i.test(path) || path.startsWith('blob:') || path.startsWith('data:')) {
    return path;
  }
  const base = apiBaseURL();
  if (!base) {
    return path;
  }
  return `${base}${path.startsWith('/') ? path : `/${path}`}`;
}

export function publicURL(path?: string): string {
  if (!path) {
    return '';
  }
  if (/^https?:\/\//i.test(path)) {
    return path;
  }
  const base = apiBaseURL() || window.location.origin;
  return `${base}${path.startsWith('/') ? path : `/${path}`}`;
}

export function wsURL(path: string): string {
  const normalizedPath = path.startsWith('/') ? path : `/${path}`;
  const base = apiBaseURL();
  if (base) {
    const url = new URL(normalizedPath, base);
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
    return url.toString();
  }

  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${window.location.host}${normalizedPath}`;
}

export function loginURL(): string {
  return window.location.protocol === 'file:' ? './index.html' : '/login';
}
