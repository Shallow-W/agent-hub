import type { ApiResponse } from '@/types/api';

const TOKEN_KEY = 'agenthub_token';

function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: number,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

const MAX_RETRY = 1;
const RETRY_DELAY_MS = 1000;

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  retryCount = 0,
): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  const token = getToken();
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const res = await fetch(path, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });

  const json: ApiResponse<T> = await res.json();

  if (!res.ok || json.code !== 0) {
    // Retry once on 5xx errors
    if (res.status >= 500 && retryCount < MAX_RETRY) {
      await new Promise((resolve) => setTimeout(resolve, RETRY_DELAY_MS));
      return request<T>(method, path, body, retryCount + 1);
    }
    throw new ApiError(res.status, json.code, json.message);
  }

  return json.data as T;
}

export function get<T>(path: string): Promise<T> {
  return request<T>('GET', path);
}

export function post<T>(path: string, body?: unknown): Promise<T> {
  return request<T>('POST', path, body);
}

export function put<T>(path: string, body?: unknown): Promise<T> {
  return request<T>('PUT', path, body);
}

export function del<T>(path: string): Promise<T> {
  return request<T>('DELETE', path);
}
