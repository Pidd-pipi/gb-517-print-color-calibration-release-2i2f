import type { EntityConfig } from './domain';

export type RunState = 'setup' | 'printing' | 'proofing' | 'hold' | 'released';
export const ALL_RUN_STATE: readonly RunState[] = ['setup', 'printing', 'proofing', 'hold', 'released'];
export type DecisionType = 'release' | 'rework' | 'quarantine';
export const ALL_DECISION_TYPE: readonly DecisionType[] = ['release', 'rework', 'quarantine'];

export const ENTITY_CONFIGS: readonly EntityConfig[] = [
  { key: 'pressUnit', path: 'presses', label: '印刷设备', statuses: ['ready', 'setup', 'printing', 'maintenance'] as const },
  { key: 'printRun', path: 'runs', label: '印刷批次', statuses: ['setup', 'printing', 'proofing', 'hold', 'released'] as const },
  { key: 'colorProof', path: 'proofs', label: '色彩校样', statuses: ['captured', 'review', 'accepted', 'rejected'] as const },
  { key: 'releaseDecision', path: 'release', label: '放行决定', statuses: ['draft', 'release', 'rework', 'quarantine'] as const }
];
