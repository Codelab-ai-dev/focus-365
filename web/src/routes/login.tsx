import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useState, FormEvent } from "react";
import { useAuth } from "@/lib/auth";

export const Route = createFileRoute("/login")({ component: LoginPage });

function LoginPage() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await login(email, password);
      navigate({ to: "/" });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Error al iniciar sesión");
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <form onSubmit={onSubmit} className="w-full max-w-sm space-y-4 rounded-xl border border-ink-700 bg-ink-900 p-6">
        <h1 className="text-2xl font-extrabold">Focus 365</h1>
        <p className="text-sm text-sand-400">Inicia sesión para continuar.</p>
        <input
          aria-label="Email" type="email" placeholder="Email" value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
        />
        <input
          aria-label="Contraseña" type="password" placeholder="Contraseña" value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
        />
        {error && <p className="text-sm text-streak">{error}</p>}
        <button type="submit" className="w-full rounded-lg bg-amber-brand px-3 py-2 text-sm font-bold text-ink-950">
          Entrar
        </button>
        <p className="text-center text-xs text-sand-400">
          ¿Sin cuenta? <Link to="/register" className="text-amber-brand">Regístrate</Link>
        </p>
      </form>
    </div>
  );
}
