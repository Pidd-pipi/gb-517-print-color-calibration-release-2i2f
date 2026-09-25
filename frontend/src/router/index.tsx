import type { ReactNode } from 'react';
import { Navigate, createBrowserRouter } from 'react-router-dom';
import App from '../App';
import PressUnitPage from '../pages/PressUnitPage';
import PrintRunPage from '../pages/PrintRunPage';
import ColorProofPage from '../pages/ColorProofPage';
import ReleaseDecisionPage from '../pages/ReleaseDecisionPage';
import AuditPage from '../pages/AuditPage';
import { roleAtLeast, useAuth, type UserRole } from '../hooks/useAuth';

function RoleGuard({ minimum, children }: { minimum: UserRole; children: ReactNode }) {
  const { session, loading } = useAuth();
  if (loading) return <div className="app-loading">正在核验访问权限…</div>;
  return roleAtLeast(session?.role, minimum) ? children : <Navigate to="/presses" replace />;
}
export const router = createBrowserRouter([{ path: '/', element: <App />, children: [
  { index: true, element: <Navigate to="/presses" replace /> },
  { path: 'presses', element: <PressUnitPage /> }, { path: 'runs', element: <PrintRunPage /> }, { path: 'proofs', element: <ColorProofPage /> }, { path: 'release', element: <ReleaseDecisionPage /> },
  { path: 'audit', element: <RoleGuard minimum="reviewer"><AuditPage /></RoleGuard> },
] }], { future: { v7_fetcherPersist: true, v7_normalizeFormMethod: true, v7_partialHydration: true, v7_relativeSplatPath: true, v7_skipActionErrorRevalidation: true } });
