import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
      // El módulo virtual del plugin PWA no existe en Vitest; lo aliaseamos a un
      // stub para que el import resuelva (los tests lo sobreescriben con vi.mock).
      "virtual:pwa-register/react": path.resolve(
        __dirname,
        "./src/test/pwa-register-stub.ts"
      ),
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/setupTests.ts"],
  },
});
