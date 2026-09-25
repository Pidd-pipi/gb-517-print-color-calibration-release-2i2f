
import type { DomainRecord } from '../../types/domain';
import { StatusBadge } from './StatusBadge';
export function ColorTable({ records, title = '色彩读数与证据' }: { records: DomainRecord[]; title?: string }) {
  if (!records.length) return null;
  return <section className="color-panel" aria-label={title}><header><div><span className="eyebrow">COLOR EVIDENCE</span><h2>{title}</h2></div><small>ΔE/密度读数随版本留痕</small></header><div className="color-table"><table><thead><tr><th>样本</th><th>读数</th><th>状态</th><th>关联批次</th><th>证据</th></tr></thead><tbody>{records.slice(0, 5).map((item) => <tr key={item.id}><td><strong>{item.code}</strong><small>{item.name}</small></td><td>{item.metricValue} {item.metricUnit}</td><td><StatusBadge status={item.status} /></td><td>{item.relatedCode || '-'}</td><td className="evidence-cell">{item.evidence || '未上传'}</td></tr>)}</tbody></table></div></section>;
}
