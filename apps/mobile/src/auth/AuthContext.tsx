import { useQueryClient } from '@tanstack/react-query';
import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { Platform } from 'react-native';

import { ApiError, setAuthFailureListener } from '@/api/client';
import { AuthApi, NotificationApi } from '@/api/services';
import {
  clearTokens,
  currentTokens,
  getItem,
  loadTokens,
  registeredPushToken,
  removeItem,
  saveTokens,
  setItem,
} from '@/api/tokens';
import type { AuthResponse, LoginRequest, RegisterRequest, User } from '@/api/types';
import { registerForPushAsync } from '@/lib/push';

type Status = 'loading' | 'authed' | 'guest';

interface AuthState {
  status: Status;
  user: User | null;
  login: (req: LoginRequest) => Promise<void>;
  register: (req: RegisterRequest) => Promise<void>;
  logout: () => Promise<void>;
  setUser: (u: User) => void;
}

const AuthContext = createContext<AuthState | null>(null);
const USER_KEY = 'billsplit.user';

function parseUser(raw: string | null): User | null {
  if (!raw) return null;
  try {
    const u = JSON.parse(raw) as Partial<User>;
    if (typeof u.id === 'string') {
      return {
        id: u.id,
        email: u.email ?? '',
        name: u.name ?? '',
        currency: u.currency ?? 'USD',
        createdAt: u.createdAt ?? null,
        payment: u.payment ?? null,
      };
    }
  } catch {
    // ignore
  }
  return null;
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const qc = useQueryClient();
  const [status, setStatus] = useState<Status>('loading');
  const [user, setUserState] = useState<User | null>(null);

  const setUser = useCallback((u: User) => {
    setUserState(u);
    void setItem(USER_KEY, JSON.stringify(u));
  }, []);

  const logout = useCallback(async () => {
    // Best effort: tell the server to forget this device and revoke the refresh token.
    const tokens = currentTokens();
    const pushToken = await registeredPushToken.get();
    if (tokens) {
      if (pushToken) await NotificationApi.unregisterDevice({ token: pushToken }).catch(() => undefined);
      await AuthApi.logout({ refreshToken: tokens.refreshToken }).catch(() => undefined);
    }
    await registeredPushToken.clear();
    await clearTokens();
    await removeItem(USER_KEY);
    qc.clear();
    setUserState(null);
    setStatus('guest');
  }, [qc]);

  // Restore session on start.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      const tokens = await loadTokens();
      if (!tokens) {
        if (!cancelled) setStatus('guest');
        return;
      }
      const cached = parseUser(await getItem(USER_KEY));
      if (cached && !cancelled) {
        setUserState(cached);
        setStatus('authed');
      }
      try {
        const me = await AuthApi.me();
        if (!cancelled) {
          setUser(me);
          setStatus('authed');
        }
      } catch (err) {
        if (cancelled) return;
        if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
          await logout();
        } else if (!cached) {
          // Offline and nothing cached: still let the user in; screens show errors with retry.
          setUserState({ id: '', email: '', name: '', currency: 'USD', createdAt: null, payment: null });
          setStatus('authed');
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [logout, setUser]);

  useEffect(() => {
    setAuthFailureListener(() => {
      void logout();
    });
    return () => setAuthFailureListener(null);
  }, [logout]);

  // Register this device for push once signed in (native only; no-op on web/simulators).
  useEffect(() => {
    if (status !== 'authed') return;
    let cancelled = false;
    void (async () => {
      const token = await registerForPushAsync();
      if (!token || cancelled) return;
      try {
        await NotificationApi.registerDevice({ token, platform: Platform.OS === 'ios' ? 'ios' : 'android' });
        await registeredPushToken.set(token);
      } catch (err) {
        console.warn('[push] RegisterDevice failed:', err);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [status]);

  const applyAuth = useCallback(
    async (res: AuthResponse) => {
      await saveTokens({ accessToken: res.accessToken, refreshToken: res.refreshToken });
      qc.clear();
      const u = res.user ?? (await AuthApi.me());
      setUser(u);
      setStatus('authed');
    },
    [qc, setUser],
  );

  const login = useCallback(async (req: LoginRequest) => applyAuth(await AuthApi.login(req)), [applyAuth]);
  const register = useCallback(
    async (req: RegisterRequest) => applyAuth(await AuthApi.register(req)),
    [applyAuth],
  );

  const value = useMemo<AuthState>(
    () => ({ status, user, login, register, logout, setUser }),
    [status, user, login, register, logout, setUser],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used inside AuthProvider');
  return ctx;
}

/** Current user id ('' while unknown). */
export function useMyId(): string {
  return useAuth().user?.id ?? '';
}
