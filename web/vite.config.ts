import { defineConfig } from "vitest/config";
import vue from "@vitejs/plugin-vue";
export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      "/api/v1": { target: "http://127.0.0.1:19080", changeOrigin: true },
    },
  },
  test: { environment: "jsdom" },
  build: { outDir: process.env.MIHOMO_WEB_DIST || "dist", emptyOutDir: true },
});
