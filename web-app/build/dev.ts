// Bun dev server with HMR: `bun run dev` (WEB_BUILD=bun) from web-app/. Env: PORT (default 3001),
// WEB_DEV_HMR=0 (no hot swap; every source change reloads the page), NEXT_PUBLIC_* (inlined, as in build.ts).
import { mkdirSync, watch, writeFileSync } from "node:fs";
import { readFileSync } from "node:fs";
import path from "node:path";
import { createReloadHub, devHtml, parseDevEnv, PortInUseError, startDevServer } from "./dev-server";
import { ve } from "./plugins/dev";

const root = path.resolve(import.meta.dir, "..");

/** bunfig.toml's [serve.static] plugins are read from the working directory, so run from web-app/. */
async function reexecInRoot(): Promise<never> {
  const child = Bun.spawn([process.execPath, path.join(root, "build/dev.ts"), ...process.argv.slice(2)], {
    cwd: root,
    stdio: ["inherit", "inherit", "inherit"],
  });
  for (const sig of ["SIGINT", "SIGTERM"] as const) process.on(sig, () => child.kill(sig));
  process.exit(await child.exited);
}

function writeShell(reloadClient: boolean): string {
  const indexDir = root;
  const shellDir = path.join(root, ".build/dev");
  mkdirSync(shellDir, { recursive: true });
  const shell = path.join(shellDir, "index.html");
  writeFileSync(shell, devHtml(readFileSync(path.join(indexDir, "index.html"), "utf8"), { indexDir, fromDir: shellDir, reloadClient }));
  return shell;
}

/** Debounced recursive watch; `match` picks the files whose change should reload every open page. */
function watchSource(match: (file: string) => boolean, onChange: () => void): void {
  let timer: ReturnType<typeof setTimeout> | undefined;
  watch(path.join(root, "src"), { recursive: true }, (_event, file) => {
    if (!file || !match(file)) return;
    clearTimeout(timer);
    timer = setTimeout(onChange, 150);
  });
}

async function main(): Promise<void> {
  if (process.cwd() !== root) return reexecInRoot();
  const { port, hmr } = parseDevEnv(process.env);
  // Without hot swap nothing refreshes the page by itself; with it, VE dependants may not be re-requested (ve.fullReloadOnCssTsChange).
  const reloadOnAnyChange = !hmr;
  const reloadOnCssTs = hmr && ve.fullReloadOnCssTsChange;
  const hub = reloadOnAnyChange || reloadOnCssTs ? createReloadHub() : undefined;
  const shell = writeShell(hub !== undefined);
  const index = (await import(shell)).default;
  const server = await startDevServer({ index, publicDir: path.join(root, "public"), port, hmr, hub, apiUrl: process.env.NEXT_PUBLIC_API_URL });
  if (hub) watchSource((f) => reloadOnAnyChange || f.endsWith(".css.ts"), () => hub.broadcast());
  console.log(`web-dev: ${server.url.href} (HMR ${hmr ? "on" : "off"}${reloadOnCssTs ? ", full reload on *.css.ts" : ""})`);
}

main().catch((e) => {
  console.error(e instanceof PortInUseError ? `web-dev: ${e.message}` : e instanceof Error ? e.stack : String(e));
  process.exit(1);
});
