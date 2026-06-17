let accessToken: string | null = null;

export function setAccessToken(token: string | null) {
  accessToken = token;
}

export function getAccessToken(): string | null {
  return accessToken;
}

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

// isAuthPath identifica los endpoints de auth, donde un 401 es legítimo (login
// con credenciales malas, refresh vencido) y NO debe disparar un refresh+reintento.
function isAuthPath(path: string): boolean {
  return path.includes("/api/v1/auth/");
}

// refreshing deduplica refrescos concurrentes: varias peticiones que reciben 401
// a la vez comparten una sola llamada a /auth/refresh.
let refreshing: Promise<boolean> | null = null;

// refreshAccessToken pide un nuevo access token usando la cookie HttpOnly de
// refresh. Devuelve true si lo consiguió. Usa fetch crudo (no apiFetch) para no
// recursar. El access token vive 15 min en memoria; esto lo renueva de forma
// transparente cuando vence con la página abierta.
function refreshAccessToken(): Promise<boolean> {
  if (!refreshing) {
    refreshing = fetch("/api/v1/auth/refresh", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
    })
      .then(async (res) => {
        if (!res.ok) return false;
        const body = (await res.json()) as { access_token?: string };
        if (!body?.access_token) return false;
        setAccessToken(body.access_token);
        return true;
      })
      .catch(() => false)
      .finally(() => {
        refreshing = null;
      });
  }
  return refreshing;
}

export async function apiFetch<T = unknown>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const doFetch = () => {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      ...(options.headers as Record<string, string>),
    };
    if (accessToken) {
      headers["Authorization"] = `Bearer ${accessToken}`;
    }
    return fetch(path, { ...options, headers, credentials: "include" });
  };

  let res = await doFetch();

  // Access token vencido (TTL 15 min) → refrescar con la cookie y reintentar una
  // vez. En los endpoints de auth un 401 es legítimo, no se reintenta.
  if (res.status === 401 && !isAuthPath(path) && (await refreshAccessToken())) {
    res = await doFetch();
  }

  if (!res.ok) {
    let message = `Error ${res.status}`;
    try {
      const body = await res.json();
      if (body?.error) message = body.error;
    } catch {
      /* respuesta sin JSON */
    }
    throw new ApiError(message, res.status);
  }

  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}
