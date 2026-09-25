
export function EmptyState({ title = '暂无记录', detail = '调整筛选条件后重试' }: { title?: string; detail?: string }) {
  return <div className="empty-state"><strong>{title}</strong><span>{detail}</span></div>;
}
