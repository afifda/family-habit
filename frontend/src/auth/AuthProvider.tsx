/* eslint-disable react-refresh/only-export-components -- the colocated hook is the public context API */
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react';

import { ApiError } from '../api/errors';
import {
  authApi,
  type LoginInput,
  type RegisterInput,
  type Session,
} from '../api/client';

type AuthContextValue = {
  session: Session | null;
  loading: boolean;
  refresh: () => Promise<void>;
  login: (input: LoginInput) => Promise<Session>;
  register: (input: RegisterInput) => Promise<Session>;
  logout: () => Promise<void>;
  unlockParent: (
    input: { password: string } | { pin: string },
  ) => Promise<Session>;
  lockParent: () => Promise<Session>;
  enterChild: (input: { childId: string; pin?: string }) => Promise<Session>;
  leaveChild: () => Promise<Session>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      setSession(await authApi.session());
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) setSession(null);
      else throw error;
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh().catch(() => setSession(null));
  }, [refresh]);

  const value = useMemo<AuthContextValue>(
    () => ({
      session,
      loading,
      refresh,
      login: async (input) => {
        const next = await authApi.login(input);
        setSession(next);
        return next;
      },
      register: async (input) => {
        const next = await authApi.register(input);
        setSession(next);
        return next;
      },
      logout: async () => {
        await authApi.logout();
        setSession(null);
      },
      unlockParent: async (input) => {
        const next = await authApi.unlockParent(input);
        setSession(next);
        return next;
      },
      lockParent: async () => {
        const next = await authApi.lockParent();
        setSession(next);
        return next;
      },
      enterChild: async (input) => {
        const next = await authApi.enterChild(input);
        setSession(next);
        return next;
      },
      leaveChild: async () => {
        const next = await authApi.leaveChild();
        setSession(next);
        return next;
      },
    }),
    [loading, refresh, session],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error('useAuth must be used within AuthProvider.');
  return value;
}
