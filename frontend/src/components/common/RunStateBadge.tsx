import type { RunState } from '../../types/status';

const stateLabel: Record<RunState, string> = {
  setup: '待装版', printing: '印刷中', proofing: '校样中', hold: '已暂停', released: '已放行',
};

export function RunStateBadge({ state }: { state: RunState }) {
  return <span className={`run-state run-state--${state}`}><i />{stateLabel[state]}</span>;
}
