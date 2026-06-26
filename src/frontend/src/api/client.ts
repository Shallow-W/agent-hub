import { message } from '@/utils/message';
import type { ApiResponse } from '@/types/api';
import { STORAGE_KEYS, API_MAX_RETRY, API_RETRY_DELAY_MS } from '@/config/constants';

function getToken(): string | null {
  return localStorage.getItem(STORAGE_KEYS.TOKEN);
}

/** Build auth headers (used by both JSON client and FormData upload). */
export function getAuthHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export function setToken(token: string): void {
  localStorage.setItem(STORAGE_KEYS.TOKEN, token);
  handling401 = false;
}

export function clearToken(): void {
  localStorage.removeItem(STORAGE_KEYS.TOKEN);
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

let handling401 = false;

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  retryCount = 0,
): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...getAuthHeaders(),
  };

  let res: Response;
  try {
    res = await fetch(path, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });
  } catch {
    message.error('网络连接失败，请检查网络');
    throw new ApiError(0, 0, '网络连接失败');
  }

  let json: ApiResponse<T>;
  try {
    json = await res.json();
  } catch {
    throw new ApiError(res.status, 0, `服务器错误 (${res.status})`);
  }

  if (!res.ok || json.code !== 0) {
    // 401 → token 过期，清除并跳转登录（防并发重复）
    if (res.status === 401) {
      if (!handling401) {
        handling401 = true;
        clearToken();
        message.warning('登录已过期，请重新登录', 2, () => {
          window.location.href = '/login';
        });
      }
      throw new ApiError(res.status, json.code, json.message);
    }
    // Retry once on 5xx errors
    if (res.status >= 500 && retryCount < API_MAX_RETRY) {
      await new Promise((resolve) => setTimeout(resolve, API_RETRY_DELAY_MS));
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

export function patch<T>(path: string, body?: unknown): Promise<T> {
  return request<T>('PATCH', path, body);
}

export function del<T>(path: string): Promise<T> {
  return request<T>('DELETE', path);
}
