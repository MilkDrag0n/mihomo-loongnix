import { heroui } from "@heroui/theme";
export default {
  content: [
    "./index.html",
    "./src/**/*.{ts,tsx}",
    "./node_modules/@holosama/ui/react/**/*.tsx",
    "./node_modules/@heroui/theme/dist/**/*.{js,ts,jsx,tsx}",
  ],
  darkMode: "class",
  plugins: [
    heroui({
      themes: {
        dark: {
          colors: {
            background: "#202837",
            content1: "#1a1724",
            foreground: "#eeeef1",
            danger: { DEFAULT: "#f54180", foreground: "#fff" },
          },
        },
        light: {
          colors: { danger: { DEFAULT: "#d62365", foreground: "#fff" } },
        },
      },
    }),
  ],
};
