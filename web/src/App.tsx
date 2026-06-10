import { useQuery } from "@tanstack/react-query";

function App() {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["health"],
    queryFn: async () => {
      const res = await fetch("/api/v1/health");
      if (!res.ok) throw new Error("API no disponible");
      return res.json() as Promise<{ status: string; service: string }>;
    },
  });

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100 flex flex-col items-center justify-center gap-4">
      <h1 className="text-4xl font-bold tracking-tight">Focus 365</h1>
      <p className="text-zinc-400">Sistema personal 360° — base lista.</p>
      <div className="rounded-lg border border-zinc-800 bg-zinc-900 px-4 py-2 text-sm">
        API:{" "}
        {isLoading ? (
          <span className="text-amber-400">conectando…</span>
        ) : isError ? (
          <span className="text-red-400">sin conexión</span>
        ) : (
          <span className="text-emerald-400">{data?.status} · {data?.service}</span>
        )}
      </div>
    </div>
  );
}

export default App;
