import { post, setToken } from './client';
import type { AuthResponse, LoginRequest, RegisterRequest } from '@/types/auth';

export async function login(
  username: string,
  password: string,
): Promise<AuthResponse> {
  const body: LoginRequest = { username, password };
  const data = await post<AuthResponse>('/api/auth/login', body);
  setToken(data.token);
  return data;
}

export async function register(
  username: string,
  password: string,
): Promise<AuthResponse> {
  const body: RegisterRequest = { username, password };
  const data = await post<AuthResponse>('/api/auth/register', body);
  setToken(data.token);
  return data;
}
