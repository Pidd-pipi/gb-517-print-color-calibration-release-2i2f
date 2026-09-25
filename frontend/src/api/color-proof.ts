
import { request } from './client';
import type { DomainRecord } from '../types/domain';

export async function listColorProof(page = 1, pageSize = 20, search = '') {
  return request<DomainRecord[]>(`/proofs?page=${page}&pageSize=${pageSize}&search=${encodeURIComponent(search)}`);
}
export async function createColorProof(input: Partial<DomainRecord>) {
  return request<DomainRecord>('/proofs', { method: 'POST', body: JSON.stringify(input) });
}
export async function transitionColorProof(id: number, status: string, expectedVersion: number, reason: string) {
  return request<DomainRecord>(`/proofs/${id}/transition`, {
    method: 'POST', body: JSON.stringify({ status, expectedVersion, reason }),
  });
}
