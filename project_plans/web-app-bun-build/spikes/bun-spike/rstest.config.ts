import { defineConfig } from "@rstest/core";
import path from "node:path";
import { VanillaExtractPlugin } from "@vanilla-extract/webpack-plugin";
import { pluginReact } from "@rsbuild/plugin-react";
export default defineConfig({
  plugins: [pluginReact()],
  testEnvironment: "jsdom",
  globals: true,
  include: ["src/**/*.test.{ts,tsx}"],
  setupFiles: ["./rstest.setup.ts"],
  testTimeout: 20000,
  tools: { rspack: { plugins: [new VanillaExtractPlugin()] } },
  resolve: { alias: {
    "next/navigation": path.resolve("shims/navigation.tsx"),
    "@": path.resolve("src"),
  } },
});
