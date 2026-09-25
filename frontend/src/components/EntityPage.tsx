import { useEffect, useMemo, useState } from 'react';
import { request } from '../api/client';
import { roleAtLeast, useAuth } from '../hooks/useAuth';
import { usePagination } from '../hooks/usePagination';
import type { EntityConfig, DomainRecord } from '../types/domain';
import type { RunState } from '../types/status';
import type { EntityStore } from '../stores/factory';
import { formatDate } from '../utils/format';
import { StatusBadge } from './common/StatusBadge';
import { RunStateBadge } from './common/RunStateBadge';
import { ColorTable } from './common/ColorTable';
import { EmptyState } from './common/EmptyState';
import { MetricCard } from './common/MetricCard';
import { ConfirmDialog } from './common/ConfirmDialog';
import { UiButton } from './common/UiButton';

function decisionRunState(status: string): RunState {
  if (status === 'release') return 'released';
  if (status === 'rework' || status === 'quarantine') return 'hold';
  return 'proofing';
}

function nextPermittedStatus(config: EntityConfig, current: string, reviewer: boolean): string | null {
  const transitions: Record<string, Record<string, string | null>> = {
    pressUnit: { ready: 'setup', setup: 'printing', printing: 'maintenance', maintenance: 'printing' },
    printRun: { setup: 'printing', printing: 'proofing', proofing: reviewer ? 'released' : 'hold', hold: 'proofing', released: reviewer ? 'hold' : null },
    colorProof: { captured: 'review', review: reviewer ? 'accepted' : null, accepted: reviewer ? 'review' : null, rejected: reviewer ? 'review' : null },
    releaseDecision: { draft: reviewer ? 'release' : 'rework', release: reviewer ? 'rework' : null, rework: reviewer ? 'release' : null, quarantine: reviewer ? 'rework' : null },
  };
  return transitions[config.key]?.[current] ?? null;
}

const verdictLabel: Record<string, string> = { pass: '合格', fail: '超限' };

export function EntityPage({ config, useStore }: { config: EntityConfig; useStore: EntityStore }) {
  const { session } = useAuth();
  const { items, meta, loading, error, load, createRecord, transition } = useStore();
  const [search, setSearch] = useState('');
  const [submittedSearch, setSubmittedSearch] = useState('');
  const [showCreate, setShowCreate] = useState(false);
  const [pending, setPending] = useState<{ item: DomainRecord; status: string } | null>(null);
  const [detail, setDetail] = useState<DomainRecord | null>(null);
  const { page, pageSize, pages, setPage, previous, next } = usePagination(meta.total);
  const canWrite = roleAtLeast(session?.role, 'operator');
  const canReview = roleAtLeast(session?.role, 'reviewer');

  useEffect(() => { void load(config.path, submittedSearch, page, pageSize); }, [config.path, load, page, pageSize, submittedSearch]);
  const highRisk = useMemo(() => items.filter((item) => ['high', 'critical'].includes(item.riskLevel)), [items]);

  // Resolve a suitable batch for the demo proof / decision so creation
  // respects the server-side gate (decisions need a passed-proof batch).
  const [runChoices, setRunChoices] = useState<DomainRecord[]>([]);
  useEffect(() => {
    if (config.key !== 'colorProof' && config.key !== 'releaseDecision') { setRunChoices([]); return; }
    void request<DomainRecord[]>('/runs?page=1&pageSize=100').then((res) => setRunChoices(res.data)).catch(() => setRunChoices([]));
  }, [config.key]);
  const defaultRunId = useMemo(() => {
    if (config.key === 'releaseDecision') {
      return runChoices.find((run) => run.status === 'released' && run.proofVerdict === 'pass')?.id
        ?? runChoices.find((run) => run.proofVerdict === 'pass')?.id ?? runChoices[0]?.id ?? null;
    }
    if (config.key === 'colorProof') {
      return runChoices.find((run) => run.status === 'proofing' || run.status === 'hold')?.id ?? runChoices[0]?.id ?? null;
    }
    return null;
  }, [runChoices, config.key]);

  const createDemo = async () => {
    const now = Date.now();
    await createRecord(config.path, {
      code: `${config.key.toUpperCase()}-${now.toString().slice(-6)}`, name: `新增${config.label}`,
      description: '通过前端工作台创建的业务记录', facility: '默认作业区', owner: session?.username || 'operator', category: '常规', riskLevel: 'medium',
      metricValue: config.key === 'colorProof' ? 1.8 : 2.4, metricUnit: 'ΔE', effectiveAt: new Date().toISOString(),
      evidence: '已完成创建前色彩检查', relatedCode: 'PR-001',
      // Batches carry a ΔE tolerance; proofs/decisions attach to a batch so the
      // gate workflow is exercisable end to end.
      ...(config.key === 'printRun' ? { colorTolerance: 3.0 } : { printRunId: defaultRunId }),
    });
    setShowCreate(false);
  };
  const openDetail = async (item: DomainRecord) => {
    try { setDetail((await request<DomainRecord>(`/${config.path}/${item.id}`)).data); }
    catch { setDetail(item); }
  };

  const confirmTransition = async (entry: { item: DomainRecord; status: string }) => {
    // Accepting a proof drives the linked batch. Send the batch version as the
    // reviewer sees it so a concurrent batch change aborts the whole review.
    if (config.key === 'colorProof' && entry.status === 'accepted') {
      const runId = entry.item.printRunId;
      if (!runId) { setPending(null); return; }
      try {
        const run = (await request<DomainRecord>(`/runs/${runId}`)).data;
        await transition(config.path, entry.item, entry.status, { expectedRunVersion: run.version });
      } catch { /* surfaced through store error */ }
      setPending(null);
      return;
    }
    await transition(config.path, entry.item, entry.status);
    setPending(null);
  };

  const actionFor = (item: DomainRecord): { label: string; status: string; disabled?: boolean; hint?: string } | null => {
    const target = nextPermittedStatus(config, item.status, canReview);
    if (!target) return null;
    if (config.key === 'printRun' && target === 'released' && item.proofVerdict !== 'pass') {
      return { label: '校样未合格', status: target, disabled: true, hint: '需合格校样才能放行' };
    }
    if (config.key === 'releaseDecision' && target === 'release' && item.status === 'draft') {
      const run = runChoices.find((r) => r.id === item.printRunId);
      if (!run || run.status !== 'released') {
        return { label: '批次未放行', status: target, disabled: true, hint: '关联批次须先放行' };
      }
    }
    return { label: `推进至 ${target}`, status: target };
  };

  return <main className="workspace">
    <header className="page-header"><div><p className="eyebrow">业务工作台</p><h1>{config.label}</h1><p>统一管理{config.label}的状态、风险、证据与责任人。</p></div>{canWrite && <UiButton onClick={() => setShowCreate(true)}>新增{config.label}</UiButton>}</header>
    <section className="metrics"><MetricCard label="记录总数" value={meta.total} detail="当前筛选范围"/><MetricCard label="高风险" value={highRisk.length} detail="需要优先复核"/><MetricCard label="状态种类" value={new Set(items.map((item) => item.status)).size} detail="状态机覆盖"/></section>
    {(config.key === 'colorProof' || config.key === 'releaseDecision') && <ColorTable records={items} title={config.key === 'colorProof' ? '当前校样读数' : '放行依据读数'} />}
    <section className="toolbar"><input aria-label="搜索" placeholder={`搜索${config.label}编码或名称`} value={search} onChange={(event) => setSearch(event.target.value)} /><UiButton onClick={() => { setPage(1); setSubmittedSearch(search); }}>查询</UiButton><button className="link-button" onClick={() => { setSearch(''); setSubmittedSearch(''); setPage(1); }}>重置</button></section>
    {error && <div className="alert" role="alert">{error}</div>}
    <section className="table-shell" aria-busy={loading}><table><thead><tr><th>编码</th><th>名称</th><th>状态</th><th>风险</th><th>责任人</th><th>指标</th><th>更新时间</th><th>操作</th></tr></thead><tbody>
      {items.map((item) => {
        const action = canWrite ? actionFor(item) : null;
        return <tr key={item.id}><td><strong>{item.code}</strong></td><td><button className="record-link" onClick={() => void openDetail(item)}>{item.name}</button><small>{item.facility}</small></td><td>{config.key === 'printRun' ? <RunStateBadge state={item.status as RunState}/> : <StatusBadge status={item.status}/>} {config.key === 'printRun' && item.proofVerdict && <span className={`proof-verdict proof-verdict--${item.proofVerdict}`}>{verdictLabel[item.proofVerdict]}</span>} {config.key === 'releaseDecision' && <RunStateBadge state={decisionRunState(item.status)}/>}</td><td>{item.riskLevel}</td><td>{item.owner}</td><td>{item.metricValue} {item.metricUnit}</td><td>{formatDate(item.updatedAt)}</td><td>{action ? (action.disabled
          ? <button className="table-action is-disabled" disabled title={action.hint}>{action.label}</button>
          : <button className="table-action" onClick={() => setPending({ item, status: action.status })}>{action.label}</button>)
          : <button className="table-action" onClick={() => void openDetail(item)}>查看详情</button>}</td></tr>;
      })}
      {!items.length && !loading && <tr><td colSpan={8}><EmptyState title="没有匹配记录" detail="可清空搜索条件后重新查询" /></td></tr>}
    </tbody></table>{loading && <div className="loading">正在同步业务数据…</div>}</section>
    <footer className="pagination"><button onClick={previous} disabled={page <= 1}>上一页</button><span>第 {page} / {pages} 页</span><button onClick={next} disabled={page >= pages}>下一页</button></footer>
    <ConfirmDialog open={showCreate} title={`新增${config.label}`} onCancel={() => setShowCreate(false)} onConfirm={() => void createDemo()}><p>将创建一条包含完整责任人、风险和证据信息的演示记录。</p>{config.key === 'printRun'
      ? <p>新批次容差默认 <strong>3.0 ΔE</strong>，接收校样时按该容差判定。</p>
      : <p>将关联批次 <strong>#{defaultRunId ?? '-'}</strong>{config.key === 'releaseDecision' ? '（须为校样合格批次）' : '，接收时读取该批次当前版本。'}</p>}</ConfirmDialog>
    <ConfirmDialog open={Boolean(pending)} title="确认状态迁移" onCancel={() => setPending(null)} onConfirm={() => { if (pending) void confirmTransition(pending); }}><p>状态迁移会写入审计日志；色彩配置和放行决定同时生成不可变版本。</p><strong>{pending?.item.status} → {pending?.status}</strong>{pending && config.key === 'colorProof' && pending.status === 'accepted' && <p>接收将读取关联批次 #{pending.item.printRunId ?? '-'} 的容差：超限批次转入等待并挡住放行，合格则保持校样中。</p>}</ConfirmDialog>
    <ConfirmDialog open={Boolean(detail)} title={`${detail?.code || ''} 记录详情`} onCancel={() => setDetail(null)} onConfirm={() => setDetail(null)}>{detail && <div className="detail-content"><p>{detail.description}</p><dl><div><dt>证据</dt><dd>{detail.evidence || '-'}</dd></div><div><dt>当前版本</dt><dd>v{detail.version}</dd></div>{config.key === 'printRun' && <PrintRunGateDetail item={detail} />}{(config.key === 'colorProof' || config.key === 'releaseDecision') && <div><dt>关联批次</dt><dd>#{detail.printRunId ?? '-'}</dd></div>}</dl><ColorTable records={[detail]} title="记录色彩读数" />{detail.revisions?.length ? <div className="revision-list"><h3>版本链</h3>{detail.revisions.map((revision) => <article key={revision.id}><strong>v{revision.version} · {revision.status}</strong><span>{revision.actor} · {revision.reason}</span><code>{revision.requestId}</code>{revision.proofVerdict && <small>校样判定：{verdictLabel[revision.proofVerdict]} · 实测 {revision.proofMeasuredDeltaE} / 容差 {revision.proofTolerance} · {revision.proofActor}</small>}</article>)}</div> : null}</div>}</ConfirmDialog>
  </main>;
}

function PrintRunGateDetail({ item }: { item: DomainRecord }) {
  return <>
    <div><dt>色差容差</dt><dd>{item.colorTolerance ? `${item.colorTolerance} ΔE` : '未设置'}</dd></div>
    {item.proofVerdict && <div className={`proof-gate proof-gate--${item.proofVerdict}`}>
      <dt>最近校样判定</dt>
      <dd><strong>{verdictLabel[item.proofVerdict]}</strong> · 实测 ΔE {item.proofMeasuredDeltaE} / 容差 {item.proofTolerance}</dd>
      <dd>操作人：{item.proofActor} · 校样 #{item.proofId ?? '-'} · {item.proofDecidedAt ? formatDate(item.proofDecidedAt) : '-'}</dd>
      <dd>{item.proofVerdict === 'fail' ? '批次已转入等待，放行被拦截。' : '批次保持校样中，可建立放行决定。'}</dd>
    </div>}
  </>;
}
