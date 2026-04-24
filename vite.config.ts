import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

const host = process.env.TAURI_DEV_HOST;

export default defineConfig(async () => ({
  plugins: [react(), tailwindcss()],
  clearScreen: false,
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          const normalizedID = id.replaceAll("\\\\", "/");
          if (normalizedID.includes("/node_modules/zrender/")) {
            return "zrender";
          }
          if (normalizedID.includes("/node_modules/echarts/lib/chart/")) {
            return "echarts-charts";
          }
          if (normalizedID.includes("/node_modules/echarts/lib/component/")) {
            return "echarts-components";
          }
          if (normalizedID.includes("/node_modules/echarts/")) {
            return "echarts";
          }
          if (
            normalizedID.includes("/node_modules/react/") ||
            normalizedID.includes("/node_modules/react-dom/") ||
            normalizedID.includes("/node_modules/react-router-dom/")
          ) {
            return "react";
          }
          return undefined;
        },
      },
    },
  },
  server: {
    port: 1420,
    strictPort: true,
    host: host || false,
    hmr: host ? { protocol: "ws", host, port: 1421 } : undefined,
    watch: { ignored: ["**/src-tauri/**"] },
  },
}));
