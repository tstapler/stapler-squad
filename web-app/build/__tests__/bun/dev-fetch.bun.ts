// Starts the real dev server (port 0) over a tiny app and probes it with fetch. Prints one JSON object of results.
import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { createReloadHub, devHtml, isAddrInUse, parseDevEnv, PortInUseError, RELOAD_PATH, startDevServer } from "../../dev-server";

const work = path.join(process.cwd(), ".build", `t-dev-${process.pid}`);
rmSync(work, { recursive: true, force: true });
mkdirSync(path.join(work, "public/icons"), { recursive: true });
writeFileSync(path.join(work, "public/push-sw.js"), "// SW_MARKER\n");
writeFileSync(path.join(work, "public/icons/a.svg"), "<svg/>");
writeFileSync(path.join(work, "main.ts"), 'document.body.dataset.ok = "1";\n');
writeFileSync(path.join(work, "index.html"), '<!doctype html><html><head></head><body><script type="module" src="./main.ts"></script></body></html>');

async function readUntil(res: Response, needle: string): Promise<string> {
  const reader = res.body!.pipeThrough(new TextDecoderStream()).getReader();
  let text = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    text += value;
    if (text.includes(needle)) break;
  }
  await reader.cancel();
  return text;
}

// Set by the fake API's handler below so the test can tell a real forward happened, not just "some 2xx".
let lastEchoed: string | undefined;

function fakeApi() {
  return Bun.serve({
    port: 0,
    hostname: "localhost",
    routes: {
      "/api/client-logs": {
        POST: async (req: Request) => {
          lastEchoed = await req.text();
          return new Response(null, { status: 207 });
        },
      },
    },
  });
}

async function probeSse(base: string, hub: ReturnType<typeof createReloadHub>) {
  const stream = await fetch(`${base}${RELOAD_PATH}`);
  const reading = readUntil(stream, "event: reload");
  await new Promise((r) => setTimeout(r, 100));
  hub.broadcast();
  const sse = await reading;
  return { type: stream.headers.get("content-type"), got: sse.includes("event: reload") };
}

async function probeClientLogs(base: string) {
  const forwarded = await fetch(`${base}/api/client-logs`, { method: "POST", body: "payload" });
  return { status: forwarded.status, echoed: lastEchoed };
}

async function main(): Promise<unknown> {
  const index = (await import(path.join(work, "index.html"))).default;
  const hub = createReloadHub();
  const api = fakeApi();
  const server = await startDevServer({ index, publicDir: path.join(work, "public"), port: 0, hmr: true, hub, apiUrl: `http://localhost:${api.port}/api` });
  const noApiServer = await startDevServer({ index, publicDir: path.join(work, "public"), port: 0, hmr: false });
  const base = `http://localhost:${server.port}`;
  try {
    const deep = await fetch(`${base}/backlog?item=x`);
    const sw = await fetch(`${base}/push-sw.js`);
    const icon = await fetch(`${base}/icons/a.svg`);
    const sse = await probeSse(base, hub);
    const clientLogs = await probeClientLogs(base);
    const clientLogsNoApi = await fetch(`http://localhost:${noApiServer.port}/api/client-logs`, { method: "POST", body: "x" });
    let second: unknown = null;
    try {
      // server.port is defined once Bun.serve has bound a concrete port (never 0, which startDevServer already resolved above).
      await startDevServer({ index, publicDir: path.join(work, "public"), port: server.port as number, hmr: true });
    } catch (e) {
      second = { isPortInUse: e instanceof PortInUseError, message: (e as Error).message, addrInUse: isAddrInUse(e) };
    }
    const shell = devHtml(
      '<html><head>\n<!--FONT_PRELOAD-->\n<!--THEME_BOOT-->\n</head><body><script type="module" src="./src/main.tsx"></script></body></html>',
      { indexDir: "/app/web-app", fromDir: "/app/web-app/.build/dev", reloadClient: true },
    );
    return {
      hostname: server.hostname,
      deep: { status: deep.status, type: deep.headers.get("content-type"), hasHtml: (await deep.text()).includes("<html") },
      sw: { status: sw.status, body: await sw.text() },
      icon: { status: icon.status, body: await icon.text() },
      sse,
      clientLogs,
      clientLogsNoApi: clientLogsNoApi.status,
      second,
      shell,
      envDefault: parseDevEnv({}),
      envHmrOff: parseDevEnv({ PORT: "62875", WEB_DEV_HMR: "0" }),
      envBad: (() => {
        try {
          return parseDevEnv({ PORT: "abc" });
        } catch (e) {
          return (e as Error).message;
        }
      })(),
    };
  } finally {
    await server.stop(true);
    await noApiServer.stop(true);
    await api.stop(true);
  }
}

try {
  console.log(JSON.stringify(await main()));
} finally {
  rmSync(work, { recursive: true, force: true });
  // The HTML bundler keeps file watchers open after server.stop().
  process.exit(0);
}
