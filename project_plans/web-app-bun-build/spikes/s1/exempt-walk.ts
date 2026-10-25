// Gate S1: every file under <outdir> must sit under an auth.go exempt prefix (server/middleware/auth.go exemptPrefixes)
// or be on the explicit public-file allowlist. Usage: bun exempt-walk.ts <outdir>
import { readdirSync } from "node:fs";
import { join, relative, resolve } from "node:path";

const exemptPrefixes = ["/auth/", "/login", "/health", "/_next/", "/favicon"];
const allowlist = new Map([["/index.html", "SPA fallback document: served by StaticFileServer for any non-file path, never requested by its own URL"]]);

const outdir = resolve(process.argv[2]);
const files: string[] = [];
const walk = (d: string) => readdirSync(d, { withFileTypes: true }).forEach((e) => (e.isDirectory() ? walk(join(d, e.name)) : files.push(join(d, e.name))));
walk(outdir);

const bad: string[] = [];
const allowed: string[] = [];
let exempt = 0;
for (const f of files) {
  const url = "/" + relative(outdir, f);
  if (exemptPrefixes.some((p) => url.startsWith(p))) exempt++;
  else if (allowlist.has(url)) allowed.push(url);
  else bad.push(url);
}
console.log(JSON.stringify({ files: files.length, underExemptPrefix: exempt, allowlisted: allowed, notExempt: bad.length, notExemptSample: bad.slice(0, 10) }));
process.exit(bad.length ? 1 : 0);
