import { defineConfig } from "@rsbuild/core";
import { pluginReact } from "@rsbuild/plugin-react";
import { VanillaExtractPlugin } from "@vanilla-extract/webpack-plugin";
import path from "node:path";
const sh = (f) => path.resolve("shims", f);
export default defineConfig({
  plugins: [pluginReact({ exclude: [/\.css\.ts$/] })],
  source: {
    entry: { index: "./entry.tsx" },
    define: { "process.env.NODE_ENV": '"production"', "process.env.NEXT_PUBLIC_API_URL": "undefined", "process.env.NEXT_PUBLIC_RECONNECT_V2": "undefined", "process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING": "undefined", "process.env.NEXT_PUBLIC_OTEL_ENABLED": "undefined", "process.env.NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "undefined" },
  },
  resolve: { alias: { "next/navigation": sh("navigation.tsx"), "next/link": sh("link.tsx"), "next/dynamic": sh("dynamic.tsx"), "next/font/local": sh("font.ts"), "next/dist/client/web-vitals": sh("webvitals.ts"), "@": path.resolve("src") } },
  tools: { rspack: { plugins: [new VanillaExtractPlugin()] } },
  output: { distPath: { root: "dist-rs" }, sourceMap: { js: "source-map" } },
  html: { template: "./index-rs.html" },
});
