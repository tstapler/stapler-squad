import { virtualCssFileFilter, getSourceFromVirtualCssFile, cssFileFilter } from "@vanilla-extract/integration";
function stripNestedKeyframes(css: string) {
  let out = "", i = 0;
  while (i < css.length) {
    const j = css.indexOf("@keyframes {", i);
    if (j < 0) { out += css.slice(i); break; }
    out += css.slice(i, j); let d = 0, k = j;
    for (; k < css.length; k++) { if (css[k] === "{") d++; else if (css[k] === "}" && --d === 0) { k++; break; } }
    i = k;
  }
  return out;
}
const veMap: Record<string, string> = JSON.parse(await Bun.file(".ve/map.json").text());
const ve = { name: "ve-bun", setup(b: any) {
  b.onResolve({ filter: virtualCssFileFilter }, (a: any) => ({ path: a.path, namespace: "ve-css" }));
  b.onLoad({ filter: /.*/, namespace: "ve-css" }, async ({ path }: any) => {
    const { source, fileName } = await getSourceFromVirtualCssFile(path);
    return { contents: stripNestedKeyframes(source), loader: "css", resolveDir: import.meta.dir + "/" + fileName.replace(/\/[^/]*$/, "") };
  });
  b.onLoad({ filter: cssFileFilter }, ({ path }: any) => ({ contents: veMap[path], loader: "js", resolveDir: path.replace(/\/[^/]*$/, "") }));
} };
const t0 = performance.now();
const shim = (name: string, file: string) => ({ name, setup(b: any) { b.onResolve({ filter: new RegExp(`^${name}$`) }, () => ({ path: import.meta.dir + "/shims/" + file })); } });
const res = await Bun.build({
  entrypoints: ["./entry.tsx"], outdir: "./dist", target: "browser", splitting: true,
  minify: process.env.MIN !== "0", sourcemap: "linked", define: { "process.env.NODE_ENV": JSON.stringify("production"), "process.env.NEXT_PUBLIC_API_URL": "undefined", "process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING": "undefined", "process.env.NEXT_PUBLIC_OTEL_ENABLED": "undefined", "process.env.NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "undefined", "process.env.NEXT_PUBLIC_RECONNECT_V2": "undefined" },
  naming: { entry: "[name].[ext]", chunk: "chunks/[name]-[hash].[ext]", asset: "assets/[name]-[hash].[ext]" },
  plugins: [
    shim("next/navigation", "navigation.tsx"), shim("next/link", "link.tsx"), shim("next/dynamic", "dynamic.tsx"),
    shim("next/font/local", "font.ts"), shim("next/dist/client/web-vitals", "webvitals.ts"),
    ve,
  ],
});
console.log(res.success ? "OK" : "FAIL", ((performance.now() - t0) / 1000).toFixed(1) + "s");
for (const l of res.logs.slice(0, 15)) console.log(String(l).slice(0, 400));
for (const o of res.outputs) console.log(o.path.split("/dist/")[1], o.size);
