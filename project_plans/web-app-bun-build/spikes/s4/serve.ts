// Static server with the Go handler's SPA semantics (server/middleware/static.go): path.Clean, existing file wins, else index.html.
// Usage: bun serve.ts [root] [port]   (ports 62875+ only; never the live instance port)
import { existsSync, statSync } from "node:fs";
import { posix } from "node:path";

const root = process.argv[2] ?? `${import.meta.dir}/dist`;
const port = Number(process.argv[3] ?? 62875);
Bun.serve({
  port,
  hostname: "127.0.0.1",
  fetch(req) {
    const clean = posix.normalize(new URL(req.url).pathname).replace(/(.)\/+$/, "$1");
    const file = `${root}${clean}`;
    if (clean !== "/" && existsSync(file) && statSync(file).isFile()) return new Response(Bun.file(file));
    return new Response(Bun.file(`${root}/index.html`), { headers: { "content-type": "text/html; charset=utf-8" } });
  },
});
console.log(`serving ${root} on ${port}`);
