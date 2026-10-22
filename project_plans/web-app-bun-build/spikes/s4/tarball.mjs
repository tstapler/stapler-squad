// Task 0.5c: tarball inspection for react-router@7.18.4 and the two transitive deps bun.lock resolves for it.
// Usage: node tarball.mjs  (needs npm + network). Exit 0 only if every package passes.
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const SPECS = ["react-router@7.18.4", "cookie@1.1.1", "set-cookie-parser@2.7.2"];
// Versions the plan names as unsafe (research/stack.md, May 2026 TanStack compromise); hook names per npm lifecycle docs.
const BANNED = { "@tanstack/react-router": ["1.169.5", "1.169.8"], "@tanstack/router-core": ["1.169.5", "1.169.8"] };
const INSTALL_HOOKS = ["preinstall", "install", "postinstall"]; // the hooks a registry consumer runs (npm and Bun)
const LOCAL_ONLY_HOOKS = ["prepare", "prepublish", "prepublishOnly"]; // not run for registry dependencies; reported, not failed
const REMOTE_SOURCE = /^(git|github|https?:)/;

function inspect(spec) {
  const dir = mkdtempSync(join(tmpdir(), "s4-tarball-"));
  const tgz = execFileSync("npm", ["pack", spec, "--silent"], { cwd: dir }).toString().trim();
  execFileSync("tar", ["xzf", tgz], { cwd: dir });
  const pkg = JSON.parse(readFileSync(join(dir, "package", "package.json"), "utf8"));
  const meta = JSON.parse(execFileSync("npm", ["view", spec, "--json", "dist.integrity", "deprecated"]).toString() || "{}");
  const hostPrefixed = JSON.stringify(pkg).match(/"[a-z]+:[^"]*"/g)?.filter((m) => /"github:/.test(m)) ?? [];
  const report = {
    spec,
    version: pkg.version,
    optionalDependencies: pkg.optionalDependencies ?? null,
    dependencies: pkg.dependencies ?? null,
    hostPrefixedSources: hostPrefixed,
    remoteSourceDependencies: Object.entries({ ...pkg.dependencies, ...pkg.optionalDependencies, ...pkg.peerDependencies }).filter(([, v]) => REMOTE_SOURCE.test(v)),
    installScripts: INSTALL_HOOKS.filter((h) => pkg.scripts?.[h]),
    localOnlyHooksPresent: LOCAL_ONLY_HOOKS.filter((h) => pkg.scripts?.[h]),
    allScripts: pkg.scripts ?? null,
    deprecated: meta.deprecated ?? null,
    integrity: meta["dist.integrity"],
    onBannedList: (BANNED[pkg.name] ?? []).includes(pkg.version),
  };
  report.pass = !report.optionalDependencies && report.hostPrefixedSources.length === 0 && report.remoteSourceDependencies.length === 0 && report.installScripts.length === 0 && !report.onBannedList && !report.deprecated;
  return report;
}

const reports = SPECS.map(inspect);
console.log(JSON.stringify(reports, null, 2));
process.exit(reports.every((r) => r.pass) ? 0 : 1);
