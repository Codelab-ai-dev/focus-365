import { useRegisterSW } from "virtual:pwa-register/react";

// UpdateToast avisa cuando hay una versión nueva de la app (tocar para recargar)
// y, brevemente, cuando quedó lista para usar sin conexión. Sin nada que avisar,
// no renderiza nada.
export function UpdateToast() {
  const {
    needRefresh: [needRefresh],
    offlineReady: [offlineReady, setOfflineReady],
    updateServiceWorker,
  } = useRegisterSW();

  if (!needRefresh && !offlineReady) return null;

  return (
    <div className="fixed inset-x-0 bottom-4 z-50 flex justify-center px-4">
      <div className="flex items-center gap-3 border-2 border-ink bg-surface px-4 py-3 shadow-brutal">
        {needRefresh ? (
          <>
            <span className="text-sm font-bold">Hay una actualización</span>
            <button
              type="button"
              onClick={() => updateServiceWorker(true)}
              className="border-2 border-ink bg-accent px-3 py-1 text-sm font-bold shadow-brutal-sm"
            >
              Recargar
            </button>
          </>
        ) : (
          <>
            <span className="text-sm font-bold">
              Lista para usar sin conexión
            </span>
            <button
              type="button"
              aria-label="Cerrar"
              onClick={() => setOfflineReady(false)}
              className="border-2 border-ink bg-surface px-3 py-1 text-sm font-bold shadow-brutal-sm"
            >
              ✕
            </button>
          </>
        )}
      </div>
    </div>
  );
}
