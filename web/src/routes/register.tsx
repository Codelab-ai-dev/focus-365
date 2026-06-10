import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useState, FormEvent } from "react";
import { useAuth } from "@/lib/auth";

export const Route = createFileRoute("/register")({ component: RegisterPage });

function RegisterPage() {
  const { register } = useAuth();
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await register(email, password, name);
      navigate({ to: "/" });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Error al registrarse");
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <form onSubmit={onSubmit} className="w-full max-w-sm space-y-4 rounded-xl border border-ink-700 bg-ink-900 p-6">
        <h1 className="text-2xl font-extrabold">Crea tu cuenta</h1>
        <input aria-label="Nombre" placeholder="Nombre" value={name}
          onChange={(e) => setName(e.target.value)}
          className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand" />
        <input aria-label="Email" type="email" placeholder="Email" value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand" />
        <input aria-label="Contraseña" type="password" placeholder="Contraseña (mín. 6)" value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand" />
        {error && <p className="text-sm text-streak">{error}</p>}
        <button type="submit" className="w-full rounded-lg bg-amber-brand px-3 py-2 text-sm font-bold text-ink-950">
          Crear cuenta
        </button>
        <p className="text-center text-xs text-sand-400">
          ¿Ya tienes cuenta? <Link to="/login" className="text-amber-brand">Inicia sesión</Link>
        </p>
      </form>
    </div>
  );
}
