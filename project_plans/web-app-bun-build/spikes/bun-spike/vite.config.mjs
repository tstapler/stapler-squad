import { defineConfig } from "vite";
import { vanillaExtractPlugin } from "@vanilla-extract/vite-plugin";
import path from "node:path";
const sh = (f) => path.resolve("shims", f);
export default defineConfig({
  plugins: [vanillaExtractPlugin()],
  esbuild: { jsx: "automatic" },
  resolve: { alias: [
    { find: /^next\/navigation$/, replacement: sh("navigation.tsx") }, { find: /^next\/link$/, replacement: sh("link.tsx") },
    { find: /^next\/dynamic$/, replacement: sh("dynamic.tsx") }, { find: /^next\/font\/local$/, replacement: sh("font.ts") },
    { find: /^next\/dist\/client\/web-vitals$/, replacement: sh("webvitals.ts") }, { find: /^@\//, replacement: path.resolve("src") + "/" } ] },
  define: { "process.env.NODE_ENV": '"production"', "process.env.NEXT_PUBLIC_API_URL": "undefined", "process.env.NEXT_PUBLIC_RECONNECT_V2": "undefined", "process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING": "undefined", "process.env.NEXT_PUBLIC_OTEL_ENABLED": "undefined", "process.env.NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "undefined" },
  build: { outDir: "dist-vite", rollupOptions: { input: "index-vite.html" }, sourcemap: true },
});
