import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import flowbiteReact from "flowbite-react/plugin/vite";

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss(), flowbiteReact()],
  server: {
    port: 5173,
  },
  test: {
    // 👋 add the line below to add jsdom to vite
    environment: "jsdom",
    globals: true,
    setupFiles: "./tests/setup.js",
    teardownTimeout: 1000,
    coverage: {
      reporter: ["text", "html", "lcov"],
    },
  },
  build: {
    // generates .vite/manifest.json in outDir
    manifest: true,
    emptyOutDir: false,
    rollupOptions: {
      // overwrite default .html entry
      input: "/src/main.jsx",
    },
  },
});
