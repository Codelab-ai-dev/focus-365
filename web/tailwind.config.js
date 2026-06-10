/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: {
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
        money: "#5ca86b",
        streak: "#e8763e",
      },
      fontFamily: {
        sans: ["Inter", "system-ui", "sans-serif"],
      },
    },
  },
  plugins: [],
};
