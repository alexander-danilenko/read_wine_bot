import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The Go binary embeds the build from internal/webapp/dist (go:embed cannot read parent folders).
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../internal/webapp/dist",
    emptyOutDir: true,
    // One bundle of React + MUI (about 160 kB gzipped) loads once and is cached as immutable.
    chunkSizeWarningLimit: 600,
  },
});
