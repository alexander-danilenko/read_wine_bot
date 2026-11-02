import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The Go binary embeds the build from internal/webapp/dist (go:embed cannot read parent folders).
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "../internal/webapp/dist",
    emptyOutDir: true,
  },
});
