// Supply-chain screen for the router dependency (Story 4.1). Reads bun.lock (JSONC) and fails on a banned
// version anywhere in the lockfile, or on a git/github source, optionalDependencies or install script in the
// closure of the watched roots. Install scripts are not in the lockfile, so they are read from installed manifests.
import { existsSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const WATCHED_ROOTS = ["react-router"];

/** Versions that were published malicious; a range-resolved install must never land on them. */
export const BANNED = [
  {
    name: "@tanstack/react-router",
    versions: ["1.169.5", "1.169.8"],
    reason: "published in the 2026-05-11 TanStack supply-chain incident (GHSA-g7cv-rxg3-hmpx)",
  },
];

const INSTALL_SCRIPTS = ["preinstall", "install", "postinstall"];
const REMOTE_SOURCE = /@(github:|git\+|git:|https?:)/;

/** bun.lock is JSONC: drop trailing commas outside strings, then parse. */
export function parseBunLock(text) {
  let out = "";
  let inString = false;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (inString) {
      out += ch;
      if (ch === "\\") out += text[++i];
      else if (ch === '"') inString = false;
    } else if (ch === '"') {
      inString = true;
      out += ch;
    } else if (ch === ",") {
      if (!/^\s*[}\]]/.test(text.slice(i + 1, i + 40))) out += ch;
    } else {
      out += ch;
    }
  }
  return JSON.parse(out);
}

function splitSpec(spec) {
  const at = spec.indexOf("@", 1);
  return { name: spec.slice(0, at), version: spec.slice(at + 1) };
}

/** Returns the problems (empty when clean) and the screened package specs. */
export function screen(lock, { roots = WATCHED_ROOTS, readManifest = () => null } = {}) {
  const problems = [];
  const entries = new Map();
  for (const [key, value] of Object.entries(lock.packages ?? {})) {
    entries.set(key, { key, spec: value[0], meta: value[2] ?? {}, ...splitSpec(value[0]) });
  }

  for (const entry of entries.values()) {
    for (const banned of BANNED) {
      if (entry.name === banned.name && banned.versions.includes(entry.version)) {
        problems.push(`${entry.spec} is banned: ${banned.reason}`);
      }
    }
  }

  const resolve = (name, parentKey) => entries.get(`${parentKey}/${name}`) ?? entries.get(name);
  const seen = new Map();
  const queue = [];
  for (const root of roots) {
    const entry = resolve(root);
    if (entry) queue.push(entry);
    else problems.push(`${root} is not in the lockfile`);
  }

  while (queue.length > 0) {
    const entry = queue.pop();
    if (seen.has(entry.key)) continue;
    seen.set(entry.key, entry.spec);
    if (REMOTE_SOURCE.test(entry.spec)) problems.push(`${entry.spec} comes from a git/github/http source`);
    const optional = Object.keys(entry.meta.optionalDependencies ?? {});
    if (optional.length > 0) problems.push(`${entry.spec} declares optionalDependencies: ${optional.join(", ")}`);
    const scripts = readManifest(entry.name, entry.version)?.scripts ?? {};
    for (const script of INSTALL_SCRIPTS.filter((name) => scripts[name])) {
      problems.push(`${entry.spec} has a ${script} script`);
    }
    for (const dep of Object.keys(entry.meta.dependencies ?? {})) {
      const next = resolve(dep, entry.key);
      if (next) queue.push(next);
    }
  }
  return { problems, screened: [...seen.values()] };
}

function manifestReader(lockPath) {
  const store = path.join(path.dirname(lockPath), "node_modules", ".bun");
  const dirs = existsSync(store) ? readdirSync(store) : [];
  // Packages with peer dependencies get a "+<hash>" suffix in the isolated store.
  return (name, version) => {
    const prefix = `${name.replace("/", "+")}@${version}`;
    const dir = dirs.find((d) => d === prefix || d.startsWith(`${prefix}+`));
    const file = dir && path.join(store, dir, "node_modules", name, "package.json");
    return file && existsSync(file) ? JSON.parse(readFileSync(file, "utf8")) : null;
  };
}

function main(argv) {
  const lockFlag = argv.indexOf("--lock");
  const lockPath = path.resolve(
    lockFlag === -1 ? path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "bun.lock") : argv[lockFlag + 1],
  );
  const { problems, screened } = screen(parseBunLock(readFileSync(lockPath, "utf8")), {
    readManifest: manifestReader(lockPath),
  });
  if (problems.length > 0) {
    for (const problem of problems) console.error(`check-dependency-screen: ${problem}`);
    return 1;
  }
  console.log(`check-dependency-screen: ${screened.join(", ")}: no github: sources, no optionalDependencies, no install scripts`);
  return 0;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exit(main(process.argv.slice(2)));
}
