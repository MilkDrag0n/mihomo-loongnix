import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
export default defineConfig({
  optimizeDeps: { include: ["@heroui/react", "framer-motion"] },
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      "/api/v1": { target: "http://127.0.0.1:19080", changeOrigin: true },
    },
  },
  build: {
    outDir: process.env.MIHOMO_WEB_DIST || "dist",
    emptyOutDir: true,
    chunkSizeWarningLimit: 1100,
  },
});
