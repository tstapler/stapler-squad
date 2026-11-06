// Gate S1: fetch the deep link and every asset unauthenticated from a running server; check status and content type.
// Usage: bun served-check.ts <baseUrl> <outdir> [deepLink]    (https remote listener: TLS verification is disabled)
import { readdirSync } from "node:fs";
import { join, relative, resolve } from "node:path";

const base = process.argv[2];
const outdir = resolve(process.argv[3]);
const deepLink = process.argv[4] ?? "/backlog/board";
const tls = { rejectUnauthorized: false };

const get = (path: string) => fetch(base + path, { redirect: "manual", tls } as RequestInit);
const html = await get(deepLink);
const htmlBody = await html.text();
const refs = [...htmlBody.matchAll(/(?:src|href)="([^"]+)"/g)].map((m) => m[1]);

const files: string[] = [];
const walk = (d: string) => readdirSync(d, { withFileTypes: true }).forEach((e) => (e.isDirectory() ? walk(join(d, e.name)) : files.push(join(d, e.name))));
walk(outdir);
const urls = files.map((f) => "/" + relative(outdir, f)).filter((u) => u !== "/index.html");

const expected = (u: string) => (u.endsWith(".js") ? /javascript/ : u.endsWith(".css") ? /css/ : u.endsWith(".woff2") ? /font|woff|octet/ : u.endsWith(".map") ? /json|octet|plain/ : /./);
const bad: string[] = [];
const byType = new Map<string, number>();
for (const u of urls) {
  const r = await get(u);
  const ct = r.headers.get("content-type") ?? "";
  byType.set(ct.split(";")[0], (byType.get(ct.split(";")[0]) ?? 0) + 1);
  if (r.status !== 200 || /text\/html/.test(ct) || !expected(u).test(ct)) bad.push(`${u} -> ${r.status} ${ct}`);
  await r.arrayBuffer();
}
const refChecks = [];
for (const u of refs) {
  const r = await get(u);
  refChecks.push({ url: u, status: r.status, contentType: r.headers.get("content-type") });
}
console.log(
  JSON.stringify({
    base,
    deepLink,
    deepLinkStatus: html.status,
    deepLinkLocation: html.headers.get("location"),
    deepLinkContentType: html.headers.get("content-type"),
    htmlRefs: refChecks,
    assetsChecked: urls.length,
    assetsBad: bad.length,
    badSample: bad.slice(0, 8),
    contentTypes: Object.fromEntries(byType),
  }),
);
process.exit(bad.length || html.status >= 400 ? 1 : 0);
