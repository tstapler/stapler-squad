import { vanillaExtract } from "./ve-plugin";
const shim = (name: string, file: string) => ({ name, setup(b: any) { b.onResolve({ filter: new RegExp(`^${name}$`) }, () => ({ path: import.meta.dir + "/shims/" + file })); } });
const t0 = performance.now();
const res = await Bun.build({
  entrypoints: ["./entry.tsx"], outdir: "./dist2", target: "browser", splitting: true, minify: true, sourcemap: "linked",
  define: { "process.env.NODE_ENV": '"production"', "process.env.NEXT_PUBLIC_API_URL": "undefined", "process.env.NEXT_PUBLIC_RECONNECT_V2": "undefined", "process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING": "undefined", "process.env.NEXT_PUBLIC_OTEL_ENABLED": "undefined", "process.env.NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "undefined" },
  naming: { entry: "[name].[ext]", chunk: "chunks/[name]-[hash].[ext]", asset: "assets/[name]-[hash].[ext]" },
  plugins: [shim("next/navigation", "navigation.tsx"), shim("next/link", "link.tsx"), shim("next/dynamic", "dynamic.tsx"), shim("next/font/local", "font.ts"), shim("next/dist/client/web-vitals", "webvitals.ts"), vanillaExtract()],
});
console.log(res.success ? "OK" : "FAIL", ((performance.now() - t0) / 1000).toFixed(2) + "s");
for (const l of res.logs.slice(0, 5)) console.log(String(l).slice(0, 300));
