import { mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { computeOutputStats, initialJsFiles } from "../stats";
import { isAllowed, validateAllowlist, verifyOutput, type AllowlistEntry, type Budgets, type VerifyInput } from "../verify-output";

const REPO = path.resolve(__dirname, "../../..");
const THEME = '{"clean":"_c1","dark":"_d1"}';
const BUDGETS: Budgets = {
  totalJsBytes: { limit: 1000, source: "t" },
  initialJsBytes: { limit: 500, source: "t" },
  largestChunkBytes: { limit: 400, source: "t" },
  maxWallMs: { limit: 10000, source: "t" },
};
const ALLOW: AllowlistEntry[] = [{ path: "push-sw.js", reason: "worker" }];

let dirs: string[] = [];
afterEach(() => {
  for (const d of dirs) rmSync(d, { recursive: true, force: true });
  dirs = [];
});

function write(root: string, rel: string, text: string): void {
  mkdirSync(path.dirname(path.join(root, rel)), { recursive: true });
  writeFileSync(path.join(root, rel), text);
}

/** A passing tree; `mutate` breaks one thing per test. */
function tree(mutate?: (root: string) => void): { out: string; input: VerifyInput } {
  const root = mkdtempSync(path.join(os.tmpdir(), "verify-"));
  dirs.push(root);
  const out = path.join(root, "out");
  write(out, "index.html", `<html><head><script>var m=${THEME};</script><link rel="stylesheet" href="/_next/static/chunks/a.css"><script type="module" src="/_next/static/chunks/a.js"></script></head></html>`);
  write(out, "_next/static/chunks/a.css", "._c1{color:red}._d1{color:blue}");
  write(out, "_next/static/chunks/a.js", 'import "/_next/static/chunks/b.js";export const a=1;');
  write(out, "_next/static/chunks/b.js", "export const b=2;");
  write(out, "push-sw.js", "//sw");
  mutate?.(out);
  const stats = { ...computeOutputStats(out), minify: true, minifyExplicit: false, totalDurationMs: 1000 };
  return { out, input: { outDir: out, statsPath: path.join(root, "stats.json"), stats, budgets: BUDGETS, allowlist: ALLOW, enforceBytes: true } };
}

const errorsOf = (input: VerifyInput): string[] => verifyOutput(input).errors;

describe("verifyOutput", () => {
  it("passes a well-formed tree", () => {
    expect(errorsOf(tree().input)).toEqual([]);
  });

  it("fails without index.html", () => {
    const { input } = tree((o) => rmSync(path.join(o, "index.html")));
    expect(errorsOf(input).join("\n")).toContain("index.html is missing");
  });

  it("fails when an asset URL in index.html does not exist or is outside /_next/static/", () => {
    const missing = tree((o) => write(o, "index.html", `<script src="/_next/static/chunks/gone.js"></script><script>var m=${THEME};</script>`));
    expect(errorsOf(missing.input).join("\n")).toContain("does not exist on disk");
    const outside = tree((o) => write(o, "index.html", `<script src="/app.js"></script><script>var m=${THEME};</script>`));
    expect(errorsOf(outside.input).join("\n")).toContain("not under /_next/static/");
  });

  it("passes the public file only while it is on the allowlist (auth lockout guard)", () => {
    const { input } = tree();
    expect(errorsOf({ ...input, allowlist: [] }).join("\n")).toContain("push-sw.js is neither under an exempt prefix");
    expect(errorsOf(input)).toEqual([]);
  });

  it("fails a file at the root outside the exempt prefixes, such as a chunk written to assets/", () => {
    const { input } = tree((o) => write(o, "assets/x.js", "1"));
    expect(errorsOf(input).join("\n")).toContain("assets/x.js");
  });

  it("fails when a bundle imports next", () => {
    const { input } = tree((o) => write(o, "_next/static/chunks/b.js", 'import{useRouter}from"next/navigation";'));
    expect(errorsOf(input).join("\n")).toContain("imports next");
  });

  it("fails when a boot-script class is missing from the CSS", () => {
    const { input } = tree((o) => write(o, "_next/static/chunks/a.css", "._c1{color:red}"));
    expect(errorsOf(input).join("\n")).toContain("_d1");
  });

  it("fails when the stats file would ship inside out/", () => {
    const { out, input } = tree();
    expect(errorsOf({ ...input, statsPath: path.join(out, "stats.json") }).join("\n")).toContain("stats file lies inside out/");
  });

  it("warns, without failing, about an unexpected process.env and stays quiet for the guarded ones", () => {
    const { input } = tree((o) => write(o, "_next/static/chunks/b.js", "export const b=process.env.FOO+process.env.LOG;"));
    const r = verifyOutput(input);
    expect(r.errors).toEqual([]);
    expect(r.warnings).toEqual([expect.stringContaining("process.env.FOO")]);
  });

  it("checks URL literals in the /login closure against the exempt prefixes and allowlist", () => {
    const { out, input } = tree((o) => write(o, "_next/static/chunks/login.js", 'import("/lazy/webauthn.js");fetch("/api/x");import("/_next/static/chunks/b.js");'));
    const errs = errorsOf({ ...input, loginClosure: [path.join(out, "_next/static/chunks/login.js")] });
    expect(errs).toEqual([expect.stringContaining("/lazy/webauthn.js")]);
  });
});

describe("budgets and wall time", () => {
  const over = (patch: Partial<VerifyInput["stats"]>) => {
    const { input } = tree();
    return errorsOf({ ...input, stats: { ...input.stats, ...patch } }).join("\n");
  };

  it("fails each budget in turn, naming the budget, bytes and limit", () => {
    expect(over({ jsBytes: 1001 })).toMatch(/Total JS is 1001 B, over the 1000 B limit/);
    expect(over({ initialJsBytes: 501 })).toMatch(/Initial JS is 501 B, over the 500 B limit/);
    expect(over({ largestChunkBytes: 401 })).toMatch(/Largest chunk is 401 B, over the 400 B limit/);
  });

  it("fails the wall-time limit and passes below it", () => {
    expect(over({ totalDurationMs: 10001 })).toMatch(/wall time 10001 ms exceeds maxWallMs 10000 ms/);
    expect(over({ totalDurationMs: 9999 })).toBe("");
  });

  it("applies byte budgets only to the build marked for enforcement", () => {
    const { input } = tree();
    expect(errorsOf({ ...input, enforceBytes: false, stats: { ...input.stats, jsBytes: 99999 } })).toEqual([]);
  });

  it("fails an unminified build unless WEB_MINIFY=0 was explicit", () => {
    expect(over({ minify: false, minifyExplicit: false })).toContain("unminified without WEB_MINIFY=0");
    expect(over({ minify: false, minifyExplicit: true })).toBe("");
  });
});

describe("stats", () => {
  it("counts the static import closure of index.html as Initial JS and excludes dynamic imports", () => {
    const { out } = tree((o) => write(o, "_next/static/chunks/c.js", "lazy"));
    write(out, "_next/static/chunks/a.js", 'import "/_next/static/chunks/b.js";import("/_next/static/chunks/c.js");');
    const names = initialJsFiles(out).map((f) => path.basename(f)).sort();
    expect(names).toEqual(["a.js", "b.js"]);
    expect(computeOutputStats(out).chunkCount).toBe(3);
  });
});

describe("public allowlist", () => {
  const file = path.join(REPO, "server/middleware/testdata/public-allowlist.json");
  const allowlist = JSON.parse(readFileSync(file, "utf8")) as AllowlistEntry[];

  it("has a reason on every entry, and the schema check rejects an empty one", () => {
    expect(validateAllowlist(allowlist)).toEqual([]);
    expect(validateAllowlist([{ path: "x", reason: " " }])).toEqual([expect.stringContaining("empty reason")]);
    expect(validateAllowlist({})).toHaveLength(1);
  });

  it("covers every file in web-app/public, so a new public file without an entry fails", () => {
    const publicDir = path.join(REPO, "web-app/public");
    const walk = (d: string): string[] => readdirSync(d, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? walk(path.join(d, e.name)) : [path.join(d, e.name)]));
    const uncovered = walk(publicDir)
      .map((f) => path.relative(publicDir, f))
      .filter((rel) => !isAllowed(rel, allowlist));
    expect(uncovered).toEqual([]);
  });

  it("has no stale entries that match no public file", () => {
    const publicDir = path.join(REPO, "web-app/public");
    const walk = (d: string): string[] => readdirSync(d, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? walk(path.join(d, e.name)) : [path.join(d, e.name)]));
    const rels = walk(publicDir).map((f) => path.relative(publicDir, f));
    for (const e of allowlist) expect(rels.some((r) => isAllowed(r, [e]))).toBe(true);
  });
});
