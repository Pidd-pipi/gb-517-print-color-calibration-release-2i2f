
import { NavLink, Outlet } from 'react-router-dom';
import { roleAtLeast, useAuth, type UserRole } from './hooks/useAuth';
const navigation: { to: string; label: string; minimum: UserRole }[] = [{ to: '/presses', label: '印刷设备', minimum: 'viewer' }, { to: '/runs', label: '印刷批次', minimum: 'viewer' }, { to: '/proofs', label: '色彩校样', minimum: 'viewer' }, { to: '/release', label: '放行决定', minimum: 'viewer' }, { to: '/audit', label: '审计记录', minimum: 'reviewer' }];
export default function App() {
  const { session, loading, switchRole } = useAuth();
  if (loading) return <div className="app-loading">正在建立安全会话…</div>;
  return <div className="app-shell"><aside><div className="brand"><span>CONTROL DESK</span><strong>印刷色彩批次校准放行</strong></div><nav>{navigation.filter((item) => roleAtLeast(session?.role, item.minimum)).map((item) => <NavLink key={item.to} to={item.to}>{item.label}</NavLink>)}</nav><div className="user-panel"><span>{session?.displayName || '系统管理员'}</span><small>{session?.role || 'admin'}</small><label htmlFor="role-switch">演示身份</label><select id="role-switch" aria-label="演示角色" value={session?.role || 'admin'} onChange={(event) => void switchRole(event.target.value as UserRole)}><option value="viewer">只读观察员</option><option value="operator">现场操作员</option><option value="reviewer">质量复核员</option><option value="admin">系统管理员</option></select></div></aside><section className="content"><header className="topbar"><span>运行态势</span><span className="live-dot">服务已连接</span></header><Outlet /></section></div>;
}
