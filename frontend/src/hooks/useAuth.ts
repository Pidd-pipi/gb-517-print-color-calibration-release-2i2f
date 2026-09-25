
import { useEffect } from 'react';
import { create } from 'zustand';
import { clearSession, getSession, saveSession } from '../api/client';
import { login } from '../api/auth';
import type { UserSession } from '../types/domain';

export type UserRole = 'viewer' | 'operator' | 'reviewer' | 'admin';
const roleRank: Record<UserRole, number> = { viewer: 1, operator: 2, reviewer: 3, admin: 4 };

interface AuthState {
  session: UserSession | null;
  loading: boolean;
  initialized: boolean;
  initialize: () => Promise<void>;
  switchRole: (role: UserRole) => Promise<void>;
  logout: () => void;
}

const initialSession = getSession<UserSession>();
const useAuthState = create<AuthState>((set, get) => ({
  session: initialSession, loading: !initialSession, initialized: Boolean(initialSession),
  initialize: async () => {
    if (get().initialized) return;
    const cached = getSession<UserSession>();
    if (cached) { set({ session: cached, initialized: true }); return; }
    set({ loading: true });
    try { const next = await login('admin'); saveSession(next); set({ session: next, initialized: true }); }
    finally { set({ loading: false }); }
  },
  switchRole: async (role) => {
    set({ loading: true });
    try { const next = await login(role); saveSession(next); set({ session: next, initialized: true }); }
    finally { set({ loading: false }); }
  },
  logout: () => { clearSession(); set({ session: null, initialized: false }); },
}));

export function roleAtLeast(role: string | undefined, minimum: UserRole): boolean {
  return Boolean(role && roleRank[role as UserRole] >= roleRank[minimum]);
}

export function useAuth() {
  const state = useAuthState();
  useEffect(() => { void state.initialize(); }, [state.initialize]);
  return { ...state, authenticated: Boolean(state.session?.token) };
}
