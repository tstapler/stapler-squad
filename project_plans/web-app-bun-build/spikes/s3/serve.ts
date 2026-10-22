// Static server with the Go handler's SPA semantics plus an /api proxy to a ManualInstance backend.
// Usage: bun serve.ts <root> <port> <backendPort>   (ports 62873-62880 only; never the live instance port)
// Lookup order: exact file, <path>/index.html (Next export, trailingSlash), root index.html (SPA fallback).
import { existsSync, statSync } from "node:fs";
import { posix } from "node:path";

const root = process.argv[2];
const port = Number(process.argv[3]);
const backend = `http://127.0.0.1:${process.argv[4]}`;
const LOW = 62873;
const HIGH = 62880;
if (!root || port < LOW || port > HIGH || Number(process.argv[4]) < LOW || Number(process.argv[4]) > HIGH) {
  console.error("usage: bun serve.ts <root> <port 62873-62880> <backendPort 62873-62880>");
  process.exit(1);
}

const STREAM_CAP_MS = 12000;
const isFile =(p: string) => existsSync(p) && statSync(p).isFile();
Bun.serve({
  port,
  hostname: "127.0.0.1",
  async fetch(req) {
    const url = new URL(req.url);
    if (url.pathname.startsWith("/api/") || url.pathname.startsWith("/auth/")) {
      // Streams are capped so abandoned pages cannot pin backend connections; hop-by-hop and encoding headers are
      // dropped because fetch() already decoded the body.
      const upstream = await fetch(backend + url.pathname + url.search, {
        method: req.method,
        headers: req.headers,
        body: req.body,
        signal: AbortSignal.any([req.signal, AbortSignal.timeout(STREAM_CAP_MS)]),
        duplex: "half",
      } as RequestInit).catch(() => null);
      if (!upstream) return new Response("upstream unavailable", { status: 502 });
      const headers = new Headers(upstream.headers);
      for (const h of ["content-encoding", "content-length", "transfer-encoding", "connection"]) headers.delete(h);
      return new Response(upstream.body, { status: upstream.status, headers });
    }
    const clean = posix.normalize(url.pathname).replace(/(.)\/+$/, "$1");
    const file = `${root}${clean}`;
    if (clean !== "/" && isFile(file)) return new Response(Bun.file(file));
    if (isFile(`${file}/index.html`)) return new Response(Bun.file(`${file}/index.html`), { headers: { "content-type": "text/html; charset=utf-8" } });
    return new Response(Bun.file(`${root}/index.html`), { headers: { "content-type": "text/html; charset=utf-8" } });
  },
});
console.log(`serving ${root} on ${port} -> backend ${backend}`);
