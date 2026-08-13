"use client";

import React, {
  createContext,
  useContext,
  useEffect,
  useState,
  useCallback,
} from "react";
import { getAuthStatus, logout as doLogout } from "@/lib/auth/passkey";

interface AuthState {
  /** Whether the server has passkey auth enabled at all. */
  authEnabled: boolean;
  /** Whether the current user is authenticated. */
  authenticated: boolean;
  /** Whether any passkeys are registered (false → first-time setup). */
  hasCredentials: boolean;
  /** Whether a bootstrap setup token is currently active. */
  setupActive: boolean;
  /** True while the initial status check is in flight. */
  loading: boolean;
}

interface AuthContextValue extends AuthState {
  /** Re-check auth status (call after login/register). */
  refresh: () => Promise<void>;
  /** Log out the current session. */
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<AuthState>({
    authEnabled: false,
    authenticated: false,
    hasCredentials: false,
    setupActive: false,
    loading: true,
  });

  const refresh = useCallback(async () => {
    try {
      const status = await getAuthStatus();
      setState({
        authEnabled: status.auth_enabled,
        authenticated: status.authenticated,
        hasCredentials: status.has_credentials,
        setupActive: status.setup_active,
        loading: false,
      });
    } catch {
      // Server unreachable or auth not configured – treat as not-authenticated
      setState((prev) => ({ ...prev, loading: false, authEnabled: false }));
    }
  }, []);

  const logout = useCallback(async () => {
    await doLogout();
    await refresh();
  }, [refresh]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return (
    <AuthContext.Provider value={{ ...state, refresh, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuth must be used within AuthProvider");
  }
  return ctx;
}
