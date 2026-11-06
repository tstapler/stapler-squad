#!/usr/bin/env node
// Moves direct createConnectTransport(...) callers onto getConnectTransport() from @/lib/api/transport.
//   node scripts/codemods/connect-transport-to-shared.mjs [--check|--dry-run] [dir-or-file ...]   (default: src)
// ast-grep (the pinned devDependency, never `sg`) locates the calls; this script applies the edits.
// Only { baseUrl, interceptors: [createAuthInterceptor()] } options are translated (baseUrl becomes
// getConnectTransport({ baseUrl }) unless it is getApiBaseUrl(), the factory default). Calls with any other
// option (fetch, useBinaryFormat, extra interceptors, ...) are listed as "non-default" and left untouched.
// --dry-run / --check report counts and exit 0 / 1 (1 when files would change) without writing.
import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const webApp = path.resolve(here, "../..");
const astGrep = path.join(webApp, "node_modules/.bin/ast-grep");

const FACTORY_MODULE = "@/lib/api/transport";
const isTestFile = (file) => /\.(test|spec)\.tsx?$/.test(file) || file.split(path.sep).includes("__tests__");
// The factory, the WS transport, and files that only mention the name (comment / jest mock factory).
const isSeam = (file) =>
  /src[\\/]lib[\\/](api[\\/]transport\.ts|transport[\\/]watch-ws-transport\.ts|utils[\\/]backoff\.ts)$/.test(file) ||
  /TestFixtures\.tsx?$/.test(file);

function scan(language, glob, targets) {
  const args = ["run", "-p", "createConnectTransport($$$ARGS)", "-l", language, "--json=compact", "--globs", glob, ...targets];
  const run = spawnSync(astGrep, args, { encoding: "utf8", maxBuffer: 256 * 1024 * 1024 });
  if (run.status !== 0 && run.status !== 1) throw new Error(`ast-grep failed: ${run.stderr}`);
  return run.stdout.trim() ? JSON.parse(run.stdout) : [];
}

/** Splits an object literal body on top-level commas. */
function topLevelProps(body) {
  const props = [];
  let depth = 0;
  let current = "";
  for (const ch of body) {
    if ("([{".includes(ch)) depth++;
    if (")]}".includes(ch)) depth--;
    if (ch === "," && depth === 0) {
      props.push(current.trim());
      current = "";
    } else current += ch;
  }
  if (current.trim()) props.push(current.trim());
  return props;
}

/** Replacement call text, or null when the options need the factory to grow first. */
export function translateCall(argText) {
  const arg = argText.trim();
  if (!arg.startsWith("{") || !arg.endsWith("}")) return null;
  let baseUrl = null;
  for (const prop of topLevelProps(arg.slice(1, -1))) {
    const base = prop.match(/^baseUrl(?:\s*:\s*([\s\S]+))?$/);
    if (base) baseUrl = (base[1] ?? "baseUrl").trim();
    else if (!/^interceptors\s*:\s*\[\s*createAuthInterceptor\(\)\s*\]$/.test(prop)) return null;
  }
  if (baseUrl === null || baseUrl === "getApiBaseUrl()") return "getConnectTransport()";
  return baseUrl === "baseUrl" ? "getConnectTransport({ baseUrl })" : `getConnectTransport({ baseUrl: ${baseUrl} })`;
}

const importRe = (name, mod) =>
  new RegExp(`import\\s*(?:type\\s*)?\\{([^}]*)\\}\\s*from\\s*["']${mod}["'];?[ \\t]*\\n?`);

function dropNames(source, mod, names) {
  const m = source.match(importRe(null, mod));
  if (!m) return source;
  const kept = m[1].split(",").map((s) => s.trim()).filter(Boolean).filter((s) => !names.includes(s.split(/\s+as\s+/)[0]));
  const quote = m[0].includes("'") ? "'" : '"';
  const text = kept.length ? `import { ${kept.join(", ")} } from ${quote}${mod}${quote};\n` : "";
  return source.replace(m[0], text);
}

function fixImports(source) {
  let out = source;
  if (!/createConnectTransport\(/.test(out)) out = dropNames(out, "@connectrpc/connect-web", ["createConnectTransport"]);
  // Drop config helpers that lost their last use (count outside their own import statement).
  const config = out.match(importRe(null, "@/lib/config"));
  if (config) {
    const rest = out.replace(config[0], "");
    const unused = ["getApiBaseUrl", "createAuthInterceptor"].filter((n) => !new RegExp(`\\b${n}\\b`).test(rest));
    if (unused.length) out = dropNames(out, "@/lib/config", unused);
  }
  const existing = out.match(importRe(null, FACTORY_MODULE));
  if (existing) {
    const names = new Set(existing[1].split(",").map((s) => s.trim()).filter(Boolean));
    names.add("getConnectTransport");
    return out.replace(existing[0], `import { ${[...names].sort().join(", ")} } from "${FACTORY_MODULE}";\n`);
  }
  const line = `import { getConnectTransport } from "${FACTORY_MODULE}";\n`;
  const connect = out.match(/import[^;]*from\s*["']@connectrpc\/connect["'];?\n/);
  if (connect) return out.replace(connect[0], connect[0] + line);
  const firstImport = out.search(/^import\s/m);
  return firstImport < 0 ? line + out : out.slice(0, firstImport) + line + out.slice(firstImport);
}

function rewrite(source, matches) {
  const bytes = Buffer.from(source);
  const edits = [];
  const nonDefault = [];
  for (const m of matches) {
    const argText = m.metaVariables?.multi?.ARGS?.map((a) => a.text).join("") ?? "";
    const text = translateCall(argText);
    if (text === null) nonDefault.push(`${m.file}:${m.range.start.line + 1}: ${m.text.replace(/\s+/g, " ").slice(0, 100)}`);
    else edits.push({ start: m.range.byteOffset.start, end: m.range.byteOffset.end, text });
  }
  if (!edits.length) return { next: source, converted: 0, nonDefault };
  let out = bytes;
  for (const e of edits.sort((a, b) => b.start - a.start)) {
    out = Buffer.concat([out.subarray(0, e.start), Buffer.from(e.text), out.subarray(e.end)]);
  }
  return { next: fixImports(out.toString()), converted: edits.length, nonDefault };
}

export function run(targets, { check = false } = {}) {
  const byFile = new Map();
  const found = [...scan("tsx", "*.tsx", targets), ...scan("ts", "*.ts", targets)];
  for (const m of found.filter((x) => !isTestFile(x.file) && !isSeam(x.file))) {
    byFile.set(m.file, [...(byFile.get(m.file) ?? []), m]);
  }
  const changed = [];
  const nonDefault = [];
  let converted = 0;
  for (const [file, matches] of byFile) {
    const source = readFileSync(file, "utf8");
    const result = rewrite(source, matches);
    nonDefault.push(...result.nonDefault);
    converted += result.converted;
    if (result.next !== source) {
      changed.push(file);
      if (!check) writeFileSync(file, result.next);
    }
  }
  return { callerFiles: [...byFile.keys()].sort(), changed: changed.sort(), converted, nonDefault: nonDefault.sort() };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const args = process.argv.slice(2);
  const check = args.includes("--check") || args.includes("--dry-run");
  const targets = args.filter((a) => !a.startsWith("--"));
  const r = run(targets.length ? targets : ["src"], { check });
  console.log(r.changed.join("\n"));
  if (r.nonDefault.length) console.error(`non-default options (left untouched):\n${r.nonDefault.join("\n")}`);
  console.error(
    `${r.callerFiles.length} caller file(s), ${r.converted} call(s) ${check ? "would convert" : "converted"} in ${r.changed.length} file(s), ${r.nonDefault.length} non-default`,
  );
  if (check && r.changed.length) process.exit(1);
}
