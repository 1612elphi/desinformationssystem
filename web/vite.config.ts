import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Dev server proxies the API to the FastAPI backend on :3650.
// Production build is emitted to dist/ and served by FastAPI on the same origin.
export default defineConfig({
  plugins: [react()],
  build: { outDir: "dist", sourcemap: false },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:3650",
      "/health": "http://localhost:3650",
    },
  },
});
