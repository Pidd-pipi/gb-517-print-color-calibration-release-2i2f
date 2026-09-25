
import { statusTone } from '../../utils/format';
export function StatusBadge({ status }: { status: string }) {
  return <span className={`status status--${statusTone(status)}`}>{status.replaceAll('_', ' ')}</span>;
}
