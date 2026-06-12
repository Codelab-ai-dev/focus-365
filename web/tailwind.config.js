/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        // -- Paleta vieja (rutas sin migrar; se elimina al cierre de R13) --
        ink: {
          DEFAULT: "var(--c-ink)", // token semántico nuevo
          950: "#13110f",
          900: "#1c1814",
          800: "#241f1a",
          700: "#2a2520",
        },
        sand: {
          100: "#f5ede0",
          400: "#a89b8c",
        },
        amber: { brand: "#e0a458" },
        money: {
          DEFAULT: "#5ca86b", // viejo text-money
          bg: "var(--c-money-bg)",
          fg: "var(--c-money-fg)",
        },
        streak: "#e8763e",
        // -- Tokens semánticos del rediseño --
        bg: "var(--c-bg)",
        surface: "var(--c-surface)",
        muted: "var(--c-muted)",
        accent: "var(--c-accent)",
        sky: { bg: "var(--c-sky-bg)", fg: "var(--c-sky-fg)" },
        sun: { bg: "var(--c-sun-bg)", fg: "var(--c-sun-fg)" },
        danger: { bg: "var(--c-danger-bg)", fg: "var(--c-danger-fg)" },
      },
      boxShadow: {
        brutal: "4px 4px 0 var(--c-shadow)",
        "brutal-sm": "3px 3px 0 var(--c-shadow)",
        "brutal-lg": "6px 6px 0 var(--c-shadow)",
      },
      fontFamily: {
        sans: ["Inter", "system-ui", "sans-serif"],
        display: ["Space Grotesk", "Inter", "sans-serif"],
      },
    },
  },
  plugins: [],
};
