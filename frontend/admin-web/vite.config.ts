import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  base: "/admin/",
  plugins: [react(), tailwindcss()],
  server: { proxy: { "/api": "http://localhost:8080", "/health": "http://localhost:8080" } },
});
