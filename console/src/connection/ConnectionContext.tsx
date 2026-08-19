/* eslint-disable react-refresh/only-export-components */
import { createContext, type PropsWithChildren, useContext, useEffect, useMemo, useState } from 'react';
import { activateDemoApi, activateHttpApi, deactivateConsoleApi } from '../api';
import { AuthApi, type ConsoleUser, ConnectionError } from '../api/authApi';

export type ConnectionMode = 'booting' | 'disconnected' | 'production' | 'demo';
interface ConnectionValue {
  mode: ConnectionMode; serverUrl: string; user?: ConsoleUser; version?: string;
  testServer(url: string): Promise<{ url: string; version: string }>;
  sendOtp(url: string, email: string): Promise<void>;
  completeLogin(url: string, email: string, code: string): Promise<void>;
  enterDemo(): void; logout(): Promise<void>;
}

const STORAGE_KEY = 'licensehub.serverUrl';
const ConnectionContext = createContext<ConnectionValue | null>(null);

export function ConnectionProvider({ children, initialDemo = false }: PropsWithChildren<{ initialDemo?: boolean }>) {
  if (initialDemo) activateDemoApi();
  const [state, setState] = useState<Pick<ConnectionValue, 'mode' | 'serverUrl' | 'user' | 'version'>>(() => initialDemo ? { mode: 'demo', serverUrl: '' } : { mode: 'booting', serverUrl: '' });

  useEffect(() => {
    if (initialDemo) { activateDemoApi(); return; }
    const savedUrl = window.localStorage.getItem(STORAGE_KEY);
    if (!savedUrl) { deactivateConsoleApi(); setState({ mode: 'disconnected', serverUrl: '' }); return; }
    const auth = new AuthApi(savedUrl);
    void Promise.all([auth.currentUser(), auth.testConnection()]).then(([user, info]) => {
      if (!user.isAdmin) throw new ConnectionError('This account is not an administrator', 'ADMIN_REQUIRED', 403);
      activateHttpApi(auth.baseUrl); setState({ mode: 'production', serverUrl: auth.baseUrl, user, version: info.version });
    }).catch(() => { deactivateConsoleApi(); setState({ mode: 'disconnected', serverUrl: savedUrl }); });
  }, [initialDemo]);

  const value = useMemo<ConnectionValue>(() => ({
    ...state,
    async testServer(url) { const auth = new AuthApi(url); const info = await auth.testConnection(); return { url: auth.baseUrl, version: info.version }; },
    async sendOtp(url, email) { await new AuthApi(url).sendOtp(email.trim().toLowerCase()); },
    async completeLogin(url, email, code) {
      const auth = new AuthApi(url); await auth.verifyOtp(email.trim().toLowerCase(), code.trim());
      const user = await auth.currentUser();
      if (!user.isAdmin) { await auth.logout(); throw new ConnectionError('This account is not an administrator', 'ADMIN_REQUIRED', 403); }
      const info = await auth.testConnection(); window.localStorage.setItem(STORAGE_KEY, auth.baseUrl);
      activateHttpApi(auth.baseUrl); setState({ mode: 'production', serverUrl: auth.baseUrl, user, version: info.version });
    },
    enterDemo() { activateDemoApi(); setState({ mode: 'demo', serverUrl: '' }); },
    async logout() {
      if (state.mode === 'production' && state.serverUrl) { try { await new AuthApi(state.serverUrl).logout(); } catch { /* proceed locally */ } }
      window.localStorage.removeItem(STORAGE_KEY); deactivateConsoleApi(); setState({ mode: 'disconnected', serverUrl: state.serverUrl });
    },
  }), [state]);

  return <ConnectionContext.Provider value={value}>{children}</ConnectionContext.Provider>;
}

export function useConnection() { const value = useContext(ConnectionContext); if (!value) throw new Error('ConnectionProvider is required'); return value; }
