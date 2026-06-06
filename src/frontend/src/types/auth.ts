export interface User {
  id: string;
  username: string;
  avatar?: string;
  created_at: string;
}

export interface AuthResponse {
  token: string;
  user: User;
}

export interface LoginRequest {
  username: string;
  password: string;
}

export interface RegisterRequest {
  username: string;
  password: string;
}
