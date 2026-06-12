import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useState, FormEvent } from "react";
import { useAuth } from "@/lib/auth";
import { Card } from "@/ui/Card";
import { Button } from "@/ui/Button";
import { Input } from "@/ui/Input";
import { PageTransition } from "@/ui/PageTransition";

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
    <PageTransition>
      <div className="flex min-h-screen items-center justify-center p-6">
        <Card className="w-full max-w-sm p-6">
          <form onSubmit={onSubmit} className="space-y-4">
            <span className="inline-block -rotate-1 rounded-sm bg-ink px-2 py-0.5 font-display text-xl font-bold uppercase tracking-tight text-bg shadow-brutal-sm">
              Focus 365
            </span>
            <p className="text-sm text-muted">Crea tu cuenta.</p>
            <Input
              aria-label="Nombre" placeholder="Nombre" value={name}
              onChange={(e) => setName(e.target.value)}
            />
            <Input
              aria-label="Email" type="email" placeholder="Email" value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
            <Input
              aria-label="Contraseña" type="password" placeholder="Contraseña (mín. 6)" value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            {error && (
              <p className="rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-sm font-bold text-danger-fg shadow-brutal-sm">
                {error}
              </p>
            )}
            <Button type="submit" className="w-full">
              Crear cuenta
            </Button>
            <p className="text-center text-xs text-muted">
              ¿Ya tienes cuenta?{" "}
              <Link to="/login" className="font-bold text-ink underline decoration-accent decoration-2 underline-offset-2">
                Inicia sesión
              </Link>
            </p>
          </form>
        </Card>
      </div>
    </PageTransition>
  );
}
