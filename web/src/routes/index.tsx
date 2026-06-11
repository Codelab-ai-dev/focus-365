import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect } from "react";
import { useAuth } from "@/lib/auth";

export const Route = createFileRoute("/")({ component: HomePage });

function HomePage() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  if (!user) return null;

  return (
    <div className="p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-extrabold">Focus 365</h1>
        <button onClick={logout} className="text-sm text-sand-400">Salir</button>
      </header>
      <p className="mt-6 text-sand-400">
        Bienvenido, <span className="text-amber-brand">{user.name}</span>.
      </p>
      <Link
        to="/check-in"
        className="mt-4 inline-block rounded-lg bg-amber-brand px-4 py-2 text-sm font-bold text-ink-950"
      >
        Check-in de hoy
      </Link>
      <Link
        to="/finanzas"
        className="mt-4 ml-2 inline-block rounded-lg border border-ink-700 px-4 py-2 text-sm font-bold text-sand-400"
      >
        Finanzas
      </Link>
      <Link
        to="/entrenamiento"
        className="mt-4 ml-2 inline-block rounded-lg border border-ink-700 px-4 py-2 text-sm font-bold text-sand-400"
      >
        Entrenamiento
      </Link>
    </div>
  );
}
