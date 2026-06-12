/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        bg: "var(--c-bg)",
        surface: "var(--c-surface)",
        ink: "var(--c-ink)",
        muted: "var(--c-muted)",
        accent: "var(--c-accent)",
        money: { bg: "var(--c-money-bg)", fg: "var(--c-money-fg)" },
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
