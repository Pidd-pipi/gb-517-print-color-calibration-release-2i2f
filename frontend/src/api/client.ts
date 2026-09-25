
import type { ApiEnvelope } from '../types/domain';

const TOKEN_KEY = 'domain-control-session';

export function getToken(): string {
  try { return JSON.parse(localStorage.getItem(TOKEN_KEY) || '{}').token || ''; } catch { return ''; }
}
export function getSession<T>(): T | null {
  try { const value = JSON.parse(localStorage.getItem(TOKEN_KEY) || 'null'); return value?.token ? value as T : null; } catch { return null; }
}
export function saveSession(session: unknown): void { localStorage.setItem(TOKEN_KEY, JSON.stringify(session)); }
export function clearSession(): void { localStorage.removeItem(TOKEN_KEY); }

export async function request<T>(path: string, init: RequestInit = {}): Promise<ApiEnvelope<T>> {
  const headers = new Headers(init.headers);
  headers.set('Accept', 'application/json');
  if (init.body) headers.set('Content-Type', 'application/json');
  const token = getToken();
  if (token) headers.set('Authorization', `Bearer ${token}`);
  const response = await fetch(`/api${path}`, { ...init, headers });
  if (response.status === 204) return { data: undefined as T };
  const payload = await response.json().catch(() => ({ error: 'invalid_response', message: '服务返回了无法解析的响应' }));
  if (response.status === 401) clearSession();
  if (!response.ok) throw new Error(payload.message || payload.error || `HTTP ${response.status}`);
  return payload as ApiEnvelope<T>;
}
