import { Link, useRouterState } from "@tanstack/react-router";
import { useAuth } from "@/lib/auth";

const LINKS: { to: string; label: string }[] = [
  { to: "/", label: "Inicio" },
  { to: "/check-in", label: "Check-in" },
  { to: "/finanzas", label: "Finanzas" },
  { to: "/entrenamiento", label: "Entreno" },
  { to: "/disciplina", label: "Disciplina" },
  { to: "/metas", label: "Metas" },
];

// TopBar es la barra de navegación persistente. Sólo se muestra con usuario;
// en /login y /register useAuth devuelve user null y no se renderiza.
export function TopBar() {
  const { user, logout } = useAuth();
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  if (!user) return null;

  return (
    <nav className="flex items-center justify-between border-b border-ink-700 bg-ink-900 px-4 py-3">
      <div className="flex items-center gap-4">
        <Link to="/" className="text-sm font-extrabold text-amber-brand">
          Focus 365
        </Link>
        <div className="flex gap-3 text-sm">
          {LINKS.map((l) => (
            <Link
              key={l.to}
              to={l.to}
              className={
                pathname === l.to
                  ? "font-bold text-amber-brand"
                  : "text-sand-400 hover:text-sand-100"
              }
            >
              {l.label}
            </Link>
          ))}
        </div>
      </div>
      <button onClick={logout} className="text-sm text-sand-400 hover:text-sand-100">
        Salir
      </button>
    </nav>
  );
}
