// Structural and budget checks on a built out/ tree, run before the swap so a bad bundle never reaches
// server/web/dist. Pure Node (no Bun APIs): Jest drives it over fixture trees.
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import { EXEMPT_PREFIXES } from "./config";
import { BuildStats, OutputStats, walkFiles } from "./stats";

export interface Budget {
  limit: number;
  /** Names the measurement the number came from (ADR-005). */
  source: string;
}
export interface Budgets {
  totalJsBytes: Budget;
  initialJsBytes: Budget;
  largestChunkBytes: Budget;
  maxWallMs: Budget;
}
export interface AllowlistEntry {
  path: string;
  reason: string;
}
export interface VerifyInput {
  outDir: string;
  /** Must not lie inside outDir: stats are build metadata, not shipped assets. */
  statsPath: string;
  stats: Pick<BuildStats, "minify" | "minifyExplicit" | "totalDurationMs"> & OutputStats;
  budgets: Budgets;
  allowlist: AllowlistEntry[];
  /** Byte budgets apply to the minified build only (ADR-004); the unminified build gets structural checks. */
  enforceBytes: boolean;
  /** JS files reachable from the /login route's entry (from the bundler metafile), when known. */
  loginClosure?: string[];
}
export interface VerifyResult {
  errors: string[];
  warnings: string[];
}

/** Guarded third-party references that survive `define` (S1-defines): both sit behind `typeof process`. */
export const GUARDED_PROCESS_ENV = ["process.env.BUF_BIGINT_DISABLE", "process.env.LOG"];

const BUILT_IN_ALLOWED = [{ path: "index.html", reason: "SPA shell; StaticFileServer serves it as the fallback for every deep link" }];

export function validateAllowlist(raw: unknown): string[] {
  if (!Array.isArray(raw)) return ["allowlist must be a JSON array of {path, reason}"];
  const errors: string[] = [];
  raw.forEach((e, i) => {
    if (typeof e?.path !== "string" || e.path === "") errors.push(`allowlist[${i}]: missing path`);
    if (typeof e?.reason !== "string" || e.reason.trim() === "") errors.push(`allowlist[${i}] (${e?.path}): empty reason`);
  });
  return errors;
}

function globToRegExp(glob: string): RegExp {
  const body = glob.replace(/[.+^${}()|[\]\\]/g, "\\$&").replace(/\*/g, "[^/]*");
  return new RegExp(`^${body}$`);
}

export function isAllowed(rel: string, allowlist: AllowlistEntry[]): boolean {
  if (EXEMPT_PREFIXES.some((p) => rel.startsWith(p))) return true;
  return [...allowlist, ...BUILT_IN_ALLOWED].some((e) => globToRegExp(e.path).test(rel));
}

const ASSET_REF = /<(?:script|link|img|source)\b[^>]*?\s(?:src|href)=["']([^"']+)["']/g;

function assetUrls(html: string): string[] {
  return [...html.matchAll(ASSET_REF)].map((m) => m[1]).filter((u) => !u.startsWith("data:") && !/^https?:/.test(u));
}

function checkIndexHtml(outDir: string, errors: string[]): string {
  const htmlPath = path.join(outDir, "index.html");
  if (!existsSync(htmlPath)) {
    errors.push("structure: out/index.html is missing");
    return "";
  }
  const html = readFileSync(htmlPath, "utf8");
  for (const url of assetUrls(html)) {
    if (!url.startsWith("/_next/static/")) errors.push(`structure: index.html asset ${url} is not under /_next/static/`);
    else if (!existsSync(path.join(outDir, url.split(/[?#]/)[0]))) errors.push(`structure: index.html asset ${url} does not exist on disk`);
  }
  return html;
}

function checkThemeBoot(html: string, css: string, errors: string[]): void {
  const m = /var m=(\{[^}]*\});/.exec(html);
  if (!m) {
    errors.push("theme: no theme boot script (var m={...}) found in index.html");
    return;
  }
  for (const cls of Object.values(JSON.parse(m[1]) as Record<string, string>)) {
    if (!css.includes(`.${cls}`)) errors.push(`theme: boot script class ${cls} is not defined in any emitted CSS`);
  }
}

const NEXT_IMPORT = /(?:\bfrom|\bimport\s*\(?|\brequire\s*\()\s*["']next(?:\/[^"']*)?["']/;
const URL_IN_CODE = /(?:import\s*\(|\bsrc\s*[:=]|\bhref\s*[:=]|\bfetch\s*\()\s*["'`](\/[^"'`\s?#]*)/g;
const STATIC_EXT = /\.(?:js|mjs|css|woff2?|png|jpe?g|svg|ico|webp|json|wasm|map)$/;

function checkLoginClosure(files: string[], allowlist: AllowlistEntry[], errors: string[]): void {
  for (const file of files) {
    for (const m of readFileSync(file, "utf8").matchAll(URL_IN_CODE)) {
      const rel = m[1].slice(1);
      // /api/* and /auth/* are RPC endpoints, not static assets; only file-like URLs can fall outside the exempt prefixes.
      if (STATIC_EXT.test(rel) && !isAllowed(rel, allowlist)) errors.push(`login: ${path.basename(file)} requests ${m[1]}, which is neither auth-exempt nor allowlisted`);
    }
  }
}

function checkBudgets(input: VerifyInput, errors: string[]): void {
  const { stats, budgets } = input;
  if (stats.totalDurationMs > budgets.maxWallMs.limit) {
    errors.push(`budget: wall time ${stats.totalDurationMs} ms exceeds maxWallMs ${budgets.maxWallMs.limit} ms`);
  }
  if (!stats.minify && !stats.minifyExplicit) errors.push("minify: build was unminified without WEB_MINIFY=0 (ADR-004)");
  if (!input.enforceBytes) return;
  const rows: [string, number, Budget][] = [
    ["Total JS", stats.jsBytes, budgets.totalJsBytes],
    ["Initial JS", stats.initialJsBytes, budgets.initialJsBytes],
    ["Largest chunk", stats.largestChunkBytes, budgets.largestChunkBytes],
  ];
  for (const [name, bytes, b] of rows) {
    if (bytes > b.limit) errors.push(`budget: ${name} is ${bytes} B, over the ${b.limit} B limit (${b.source})`);
  }
}

export function verifyOutput(input: VerifyInput): VerifyResult {
  const errors: string[] = [];
  const warnings: string[] = [];
  const { outDir } = input;

  const html = checkIndexHtml(outDir, errors);
  const files = walkFiles(outDir);
  for (const f of files) {
    const rel = path.relative(outDir, f).split(path.sep).join("/");
    if (!isAllowed(rel, input.allowlist)) errors.push(`auth: ${rel} is neither under an exempt prefix nor on the public allowlist (it would redirect to /login)`);
  }
  if (!path.relative(outDir, input.statsPath).startsWith("..")) errors.push("structure: stats file lies inside out/ and would ship in the binary");

  const staticJs = files.filter((f) => f.includes(`${path.sep}_next${path.sep}static${path.sep}`) && f.endsWith(".js"));
  for (const f of staticJs) {
    const text = readFileSync(f, "utf8");
    if (NEXT_IMPORT.test(text)) errors.push(`next: ${path.relative(outDir, f)} imports next`);
    for (const hit of new Set(text.match(/process\.env\.[A-Za-z0-9_]+/g) ?? [])) {
      if (!GUARDED_PROCESS_ENV.includes(hit)) warnings.push(`process.env: ${hit} remains in ${path.relative(outDir, f)}; confirm it is guarded by typeof process`);
    }
  }

  const css = files.filter((f) => f.endsWith(".css")).map((f) => readFileSync(f, "utf8")).join("\n");
  if (html) checkThemeBoot(html, css, errors);
  if (input.loginClosure) checkLoginClosure(input.loginClosure, input.allowlist, errors);
  checkBudgets(input, errors);
  return { errors, warnings };
}
