
import { request } from './client';
import type { DomainRecord } from '../types/domain';

export async function listPressUnit(page = 1, pageSize = 20, search = '') {
  return request<DomainRecord[]>(`/presses?page=${page}&pageSize=${pageSize}&search=${encodeURIComponent(search)}`);
}
export async function createPressUnit(input: Partial<DomainRecord>) {
  return request<DomainRecord>('/presses', { method: 'POST', body: JSON.stringify(input) });
}
export async function transitionPressUnit(id: number, status: string, expectedVersion: number, reason: string) {
  return request<DomainRecord>(`/presses/${id}/transition`, {
    method: 'POST', body: JSON.stringify({ status, expectedVersion, reason }),
  });
}
