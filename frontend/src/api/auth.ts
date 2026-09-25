
import { request } from './client';
import type { UserSession } from '../types/domain';
export async function login(username = 'admin', password = 'Admin123!'): Promise<UserSession> {
  return (await request<UserSession>('/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) })).data;
}
