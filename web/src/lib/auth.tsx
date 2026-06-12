import { createContext, useContext, useState, useEffect, ReactNode, useCallback } from "react";
import { apiFetch, setAccessToken } from "./api";

export type User = { id: string; email: string; name: string };
type AuthResp = { access_token: string; user: User };

type AuthState = {
  user: User | null;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, name: string) => Promise<void>;
  logout: () => void;
};

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [ready, setReady] = useState(false);

  // Al montar, intenta restaurar la sesión con la cookie HttpOnly de refresh.
  // Los children no se renderizan hasta resolver, para que los guards de ruta
  // no redirijan a /login antes de saber si hay sesión.
  useEffect(() => {
    let cancelled = false;
    apiFetch<AuthResp>("/api/v1/auth/refresh", { method: "POST" })
      .then((resp) => {
        if (cancelled) return;
        setAccessToken(resp.access_token);
        setUser(resp.user);
      })
      .catch(() => {
        /* sin sesión previa */
      })
      .finally(() => {
        if (!cancelled) setReady(true);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    const resp = await apiFetch<AuthResp>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    });
    setAccessToken(resp.access_token);
    setUser(resp.user);
  }, []);

  const register = useCallback(async (email: string, password: string, name: string) => {
    const resp = await apiFetch<AuthResp>("/api/v1/auth/register", {
      method: "POST",
      body: JSON.stringify({ email, password, name }),
    });
    setAccessToken(resp.access_token);
    setUser(resp.user);
  }, []);

  const logout = useCallback(() => {
    void apiFetch("/api/v1/auth/logout", { method: "POST" }).catch(() => {
      /* la sesión local se limpia igual */
    });
    setAccessToken(null);
    setUser(null);
  }, []);

  return (
    <AuthContext.Provider value={{ user, login, register, logout }}>
      {ready ? children : null}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth debe usarse dentro de AuthProvider");
  return ctx;
}
