import { createContext, useContext, useState, ReactNode, useCallback } from "react";
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
    setAccessToken(null);
    setUser(null);
  }, []);

  return (
    <AuthContext.Provider value={{ user, login, register, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth debe usarse dentro de AuthProvider");
  return ctx;
}
