import { Link, useRouterState } from "@tanstack/react-router";
import { useAuth } from "@/lib/auth";
import { ThemeToggle } from "@/ui/theme";

const LINKS: { to: string; label: string }[] = [
  { to: "/", label: "Inicio" },
  { to: "/check-in", label: "Check-in" },
  { to: "/finanzas", label: "Finanzas" },
  { to: "/entrenamiento", label: "Entreno" },
  { to: "/disciplina", label: "Disciplina" },
  { to: "/metas", label: "Metas" },
  { to: "/asistente", label: "Asistente" },
];

// TopBar es la barra de navegación persistente. Sólo se muestra con usuario;
// en /login y /register useAuth devuelve user null y no se renderiza.
export function TopBar() {
  const { user, logout } = useAuth();
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  if (!user) return null;

  return (
    <nav className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b-[2.5px] border-ink bg-bg px-4 py-3">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <Link
          to="/"
          className="inline-block -rotate-1 rounded-sm bg-ink px-2 py-0.5 font-display text-sm font-bold uppercase tracking-tight text-bg shadow-brutal-sm transition-transform hover:rotate-1"
        >
          Focus 365
        </Link>
        <div className="flex flex-wrap gap-1.5 text-sm">
          {LINKS.map((l) => (
            <Link
              key={l.to}
              to={l.to}
              className={
                pathname === l.to
                  ? "rounded-md border-2 border-ink bg-accent px-2 py-0.5 font-bold text-[#16130e] shadow-brutal-sm"
                  : "rounded-md border-2 border-transparent px-2 py-0.5 font-medium text-muted transition-colors hover:border-ink hover:text-ink"
              }
            >
              {l.label}
            </Link>
          ))}
        </div>
      </div>
      <div className="flex items-center gap-3">
        <ThemeToggle />
        <button
          onClick={logout}
          className="text-sm font-medium text-muted transition-colors hover:text-ink"
        >
          Salir
        </button>
      </div>
    </nav>
  );
}
