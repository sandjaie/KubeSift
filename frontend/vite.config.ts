/*
Purpose: Configure frontend development and production bundling.
Used by: Vite, npm scripts and the desktop build.
Responsibilities: Enable React and Wails binding support and restrict the development server to loopback.
Important: Production assets are embedded in the app; do not add a shipped sidecar server.
*/
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
export default defineConfig({
  build: {outDir: "../internal/desktop/assets/dist",emptyOutDir: true},
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [react(), wails("./bindings")],
});
