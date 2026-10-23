// Pure parts of the Bun dev server (build/dev.ts): env parsing, public/ routes, the dev HTML shell and
// port-in-use reporting. Kept apart from dev.ts so Bun-side tests can start a server without the CLI wrapper.
import { spawnSync } from "node:child_process";
import { existsSync, readdirSync } from "node:fs";
import net from "node:net";
import path from "node:path";
import { FONT_PRELOAD_PLACEHOLDER, THEME_BOOT_PLACEHOLDER } from "./config";

/** Same default as `next dev --port ${PORT:-3001}`. */
export const DEFAULT_DEV_PORT = 3001;
export const DEV_HOSTNAME = "localhost";
export const RELOAD_PATH = "/_dev/reload";

export interface DevEnv {
  port: number;
  hmr: boolean;
}

export function parseDevEnv(env: Record<string, string | undefined>): DevEnv {
  const raw = env.PORT;
  const port = raw === undefined || raw === "" ? DEFAULT_DEV_PORT : Number(raw);
  if (!Number.isInteger(port) || port < 0 || port > 65535) throw new Error(`PORT must be an integer from 0 to 65535; got "${raw}"`);
  return { port, hmr: env.WEB_DEV_HMR !== "0" };
}

function walk(dir: string, base = ""): string[] {
  return readdirSync(path.join(dir, base), { withFileTypes: true }).flatMap((e) => {
    const rel = path.posix.join(base, e.name);
    return e.isDirectory() ? walk(dir, rel) : [rel];
  });
}

/** One route per public/ file, read per request so an edited service worker is served fresh. */
export function publicRoutes(publicDir: string): Record<string, () => Response> {
  if (!existsSync(publicDir)) return {};
  return Object.fromEntries(
    walk(publicDir).map((rel) => {
      const file = path.join(publicDir, rel);
      return [`/${rel}`, () => new Response(Bun.file(file))] as const;
    }),
  );
}

/** Bun inlines only the NEXT_PUBLIC_* that are set; a read of an unset one would otherwise hit `process is not defined`. */
const PROCESS_SHIM = "<script>globalThis.process=globalThis.process||{env:{}}</script>";

/**
 * Rewrites index.html into the dev shell. `fromDir` is where the shell is written; the production placeholders
 * become the dev theme-boot module (first script) and the reload client; the font preload has no dev equivalent.
 */
export function devHtml(indexHtml: string, opts: { indexDir: string; fromDir: string; reloadClient: boolean }): string {
  const rel = path.relative(opts.fromDir, opts.indexDir).split(path.sep).join("/") || ".";
  const themeBoot = `<script type="module" src="${rel}/src/theme-boot.ts"></script>`;
  const reload = opts.reloadClient ? `<script>new EventSource("${RELOAD_PATH}").addEventListener("reload",function(){location.reload()})</script>` : "";
  if (!indexHtml.includes(THEME_BOOT_PLACEHOLDER)) throw new Error(`index.html lost ${THEME_BOOT_PLACEHOLDER}; the dev shell injects theme-boot there`);
  return indexHtml
    .replace(FONT_PRELOAD_PLACEHOLDER, "")
    .replace(THEME_BOOT_PLACEHOLDER, `${PROCESS_SHIM}${themeBoot}${reload}`)
    .replace(/(src=")\.\/(src\/)/g, `$1${rel}/$2`);
}

export class PortInUseError extends Error {
  constructor(
    readonly port: number,
    readonly holder?: string,
  ) {
    super(
      `port ${port} is already in use${holder ? ` by ${holder}` : ""}.\n` +
        `Find the holder with: ss -ltnp | grep ':${port} '\n` +
        `Then stop it, or pick another port: PORT=<free port> bun run dev`,
    );
    this.name = "PortInUseError";
  }
}

/** Best effort: `ss` may be missing (macOS) or hide other users' processes. */
export function portHolder(port: number): string | undefined {
  const r = spawnSync("ss", ["-ltnpH", `sport = :${port}`], { encoding: "utf8", timeout: 3000 });
  if (r.error || r.status !== 0) return undefined;
  const m = /users:\(\("([^"]+)",pid=(\d+)/.exec(r.stdout);
  return m ? `${m[1]} (pid ${m[2]})` : undefined;
}

export function isAddrInUse(e: unknown): boolean {
  return (e as { code?: string }).code === "EADDRINUSE" || /EADDRINUSE|address already in use|in use/i.test(String((e as Error)?.message));
}

/** Server-sent-events hub: the page's reload client (devHtml) listens, and the watcher broadcasts. */
export function createReloadHub() {
  const clients = new Set<ReadableStreamDefaultController<string>>();
  return {
    get size() {
      return clients.size;
    },
    handler: (): Response => {
      let controller: ReadableStreamDefaultController<string>;
      const body = new ReadableStream<string>({
        start(c) {
          controller = c;
          clients.add(c);
          c.enqueue(": connected\n\n");
        },
        cancel() {
          clients.delete(controller);
        },
      });
      return new Response(body, { headers: { "content-type": "text/event-stream", "cache-control": "no-store" } });
    },
    broadcast(): void {
      for (const c of clients) {
        try {
          c.enqueue("event: reload\ndata: 1\n\n");
        } catch {
          clients.delete(c);
        }
      }
    },
  };
}
export type ReloadHub = ReturnType<typeof createReloadHub>;

export const CLIENT_LOGS_PATH = "/api/client-logs";

/**
 * The log shipper posts to a same-origin path, which this server cannot answer. Forward it to the API when one
 * is configured (NEXT_PUBLIC_API_URL ends in /api) so dev logs land in the backend like production; else accept and drop.
 */
export function clientLogsHandler(apiUrl: string | undefined): (req: Request) => Promise<Response> {
  return async (req) => {
    if (!apiUrl) return new Response(null, { status: 204 });
    try {
      const res = await fetch(`${apiUrl.replace(/\/+$/, "")}/client-logs`, { method: "POST", headers: req.headers, body: await req.arrayBuffer() });
      return new Response(null, { status: res.status });
    } catch {
      return new Response(null, { status: 204 });
    }
  };
}

export interface StartOptions {
  /** The imported dev HTML bundle (`(await import(shell)).default`). */
  index: unknown;
  publicDir: string;
  port: number;
  hmr: boolean;
  hub?: ReloadHub;
  /** NEXT_PUBLIC_API_URL; where /api/client-logs is forwarded. */
  apiUrl?: string;
}

function accepts(host: string, port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const s = net.connect({ host, port });
    s.setTimeout(500);
    s.once("connect", () => {
      s.destroy();
      resolve(true);
    });
    s.once("error", () => resolve(false));
    s.once("timeout", () => {
      s.destroy();
      resolve(false);
    });
  });
}

/** Bun.serve binds a port another Bun listener already holds (SO_REUSEPORT) instead of throwing EADDRINUSE, so probe by connecting. */
export async function assertPortFree(port: number): Promise<void> {
  if (port === 0) return;
  for (const host of ["127.0.0.1", "::1"]) {
    if (await accepts(host, port)) throw new PortInUseError(port, portHolder(port));
  }
}

export async function startDevServer(o: StartOptions) {
  await assertPortFree(o.port);
  try {
    return Bun.serve({
      port: o.port,
      hostname: DEV_HOSTNAME,
      // SSE streams are idle between edits; Bun's default 10 s idle timeout would drop them.
      idleTimeout: 0,
      development: { hmr: o.hmr, console: true },
      routes: {
        // Always registered (never a conditional spread key) so the route map's value type stays non-optional for Bun.serve.
        [RELOAD_PATH]: o.hub ? o.hub.handler : () => new Response(null, { status: 404 }),
        ...publicRoutes(o.publicDir),
        [CLIENT_LOGS_PATH]: { POST: clientLogsHandler(o.apiUrl) },
        "/*": o.index as never,
      },
    });
  } catch (e) {
    if (isAddrInUse(e)) throw new PortInUseError(o.port, portHolder(o.port));
    throw e;
  }
}
