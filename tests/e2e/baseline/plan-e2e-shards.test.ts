import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import { spawnSync } from "child_process";

// Runs scripts/plan-e2e-shards.sh with stubbed gh/bun/git (real jq, sed, grep) and asserts the
// `skip-baseline` output is empty on every failure path: that flag must only be 1 when the whole selection succeeded.
const script = path.resolve(__dirname, "../scripts/plan-e2e-shards.sh");
let dir: string;

const stub = (name: string, body: string) => {
  const p = path.join(dir, "bin", name);
  fs.writeFileSync(p, `#!/usr/bin/env bash\n${body}\n`, { mode: 0o755 });
};

beforeAll(() => {
  // Not os.tmpdir(): /tmp is a RAM tmpfs that fills up on busy dev hosts.
  const base = path.join(os.homedir(), ".cache", "ssq-e2e-sim");
  fs.mkdirSync(base, { recursive: true });
  dir = fs.mkdtempSync(path.join(base, "plan-shards-"));
  fs.mkdirSync(path.join(dir, "bin"));
  stub(
    "gh",
    `case "$GH_MODE" in
  ok) printf 'tests/e2e/touch-targets.spec.ts\\nserver/x.go\\n' ;;
  fail) exit 1 ;;
  partial) echo 'tests/e2e/touch-targets.spec.ts'; exit 1 ;;
  many) seq 1 3500 | sed 's#^#f#' ;;
  empty) : ;;
esac`,
  );
  stub(
    "git",
    `[ "$1" = ls-files ] || exit 0
case "$GIT_MODE" in
  fail) exit 1 ;;
  onlynm) echo 'tests/e2e/node_modules/x.spec.ts' ;;
  *) printf 'tests/e2e/touch-targets.spec.ts\\ntests/e2e/a.spec.ts\\n' ;;
esac`,
  );
  stub(
    "bun",
    `if printf '%s ' "$@" | grep -q -- '--summary'; then
  [ "$BUN_MODE" = crash1 ] && exit 1
  echo '### summary'; exit 0
fi
[ "$BUN_MODE" = crash2 ] && exit 1
[ "$BUN_MODE" = badjson ] && { echo 'not json'; exit 0; }
echo '{"mode":"subset","shards":2,"specs":["touch-targets.spec.ts"]}'`,
  );
});

afterAll(() => fs.rmSync(dir, { recursive: true, force: true }));

function run(env: Record<string, string>): { status: number | null; out: Record<string, string> } {
  const outFile = path.join(dir, "gh-output");
  fs.rmSync(outFile, { force: true });
  const tmp = fs.mkdtempSync(path.join(dir, "run-"));
  const r = spawnSync("bash", [script], {
    env: {
      PATH: `${path.join(dir, "bin")}:${process.env.PATH}`,
      E2E_SHARD_TOTAL: "5",
      EVENT_NAME: "pull_request",
      REPO: "o/r",
      PR_NUMBER: "1",
      RUNNER_TEMP: tmp,
      GITHUB_OUTPUT: outFile,
      GH_MODE: "ok",
      GIT_MODE: "ok",
      BUN_MODE: "ok",
      ...env,
    },
    encoding: "utf8",
  });
  const out: Record<string, string> = {};
  if (fs.existsSync(outFile)) {
    const lines = fs.readFileSync(outFile, "utf8").split("\n");
    for (let i = 0; i < lines.length; i++) {
      const m = lines[i].match(/^([a-z-]+)(?:=(.*)|<<(\S+))$/);
      if (!m) continue;
      if (m[3]) {
        const body: string[] = [];
        for (i++; lines[i] !== m[3]; i++) body.push(lines[i]);
        out[m[1]] = body.join("\n");
      } else out[m[1]] = m[2];
    }
  }
  return { status: r.status, out };
}

describe("plan-e2e-shards.sh", () => {
  test("PS1: all steps ok -> selection used, skip-baseline=1, changed specs listed", () => {
    const { status, out } = run({});
    expect(status).toBe(0);
    expect(out["skip-baseline"]).toBe("1");
    expect(out.mode).toBe("subset");
    expect(out.total).toBe("2");
    expect(out["changed-specs"]).toBe("touch-targets.spec.ts");
  });

  const failures: [string, Record<string, string>][] = [
    ["gh api fails with no file", { GH_MODE: "fail" }],
    ["gh api fails after a partial list", { GH_MODE: "partial" }],
    ["3500 changed files (3000 cap)", { GH_MODE: "many" }],
    ["empty changed list", { GH_MODE: "empty" }],
    ["git ls-files fails", { GIT_MODE: "fail" }],
    ["every spec filtered out", { GIT_MODE: "onlynm" }],
    ["first select-specs call crashes", { BUN_MODE: "crash1" }],
    ["second select-specs call crashes", { BUN_MODE: "crash2" }],
    ["select-specs prints invalid JSON", { BUN_MODE: "badjson" }],
  ];
  test.each(failures)("PS2: %s -> full suite, skip-baseline empty", (_name, env) => {
    const { status, out } = run(env);
    expect(status).toBe(0);
    expect(out["skip-baseline"]).toBe("");
    expect(out.mode).toBe("all");
    expect(out.total).toBe("5");
    expect(out["changed-specs"]).toBe("");
  });

  test("PS3: a non-PR event never sets skip-baseline", () => {
    const { out } = run({ EVENT_NAME: "push" });
    expect(out["skip-baseline"]).toBe("");
    expect(out.mode).toBe("all");
  });

  test("PS4: an invalid shard total fails the step", () => {
    expect(run({ E2E_SHARD_TOTAL: "x" }).status).toBe(1);
  });
});
