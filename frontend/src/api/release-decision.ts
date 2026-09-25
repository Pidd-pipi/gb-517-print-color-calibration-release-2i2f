
import { request } from './client';
import type { DomainRecord } from '../types/domain';

export async function listReleaseDecision(page = 1, pageSize = 20, search = '') {
  return request<DomainRecord[]>(`/release?page=${page}&pageSize=${pageSize}&search=${encodeURIComponent(search)}`);
}
export async function createReleaseDecision(input: Partial<DomainRecord>) {
  return request<DomainRecord>('/release', { method: 'POST', body: JSON.stringify(input) });
}
export async function transitionReleaseDecision(id: number, status: string, expectedVersion: number, reason: string) {
  return request<DomainRecord>(`/release/${id}/transition`, {
    method: 'POST', body: JSON.stringify({ status, expectedVersion, reason }),
  });
}
