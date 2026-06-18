// Stub de `virtual:pwa-register/react` para el entorno de Vitest (donde el plugin
// PWA no está cargado y el módulo virtual no se puede resolver). Los tests que
// necesiten controlar el estado lo sobreescriben con vi.mock; este stub solo
// asegura que el import resuelva.
export function useRegisterSW() {
  return {
    needRefresh: [false, () => {}] as [boolean, (v: boolean) => void],
    offlineReady: [false, () => {}] as [boolean, (v: boolean) => void],
    updateServiceWorker: (_reload?: boolean) => Promise.resolve(),
  };
}
