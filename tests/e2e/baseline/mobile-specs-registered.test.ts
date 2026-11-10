// Task 6.3.3: a `*-mobile.spec.ts` that asserts touch must run on the mobile projects, not on a resized desktop window.
import * as fs from "fs";
import * as path from "path";

const e2eDir = path.join(__dirname, "..");
const config = fs.readFileSync(path.join(e2eDir, "playwright.config.ts"), "utf8");

const MULTI_PR_MOBILE_SPECS = [
  "multi-pr-session-mobile.spec.ts",
  "artifacts-tab-mobile.spec.ts",
  "artifacts-view-mobile.spec.ts",
  "omnibar-find-pr-mobile.spec.ts",
  "backlog-import-close-issue-mobile.spec.ts",
];

// Desktop-run before this plan and unchanged by it; moving them is out of scope.
const LEGACY_MOBILE_NAMED = ["backlog-result-panels-mobile.spec.ts", "toast-stack-mobile.spec.ts"];

function arrayLiteral(name: string): string[] {
  const m = config.match(new RegExp(`const ${name} = \\[([\\s\\S]*?)\\];`));
  if (!m) throw new Error(`${name} not found in playwright.config.ts`);
  return [...m[1].matchAll(/'([^']+)'/g)].map((x) => x[1]);
}

function globToRegExp(glob: string): RegExp {
  const body = glob.replace(/^\*\*\//, "").replace(/[.+?^${}()|[\]\\]/g, "\\$&").replace(/\*/g, ".*");
  return new RegExp(`^${body}$`);
}

function projectBlock(name: string): string {
  const start = config.indexOf(`name: '${name}',`);
  if (start < 0) throw new Error(`project ${name} not found`);
  return config.slice(start, config.indexOf("use:", start));
}

describe("mobile spec registration", () => {
  const mobileOnly = arrayLiteral("MOBILE_ONLY_SPECS");

  test("MS1: the five multi-PR mobile specs are MOBILE_ONLY_SPECS patterns", () => {
    expect(mobileOnly.sort()).toEqual(MULTI_PR_MOBILE_SPECS.map((f) => `**/${f}`).sort());
  });

  test("MS2: MOBILE_SPECS includes MOBILE_ONLY_SPECS, so both mobile projects run them", () => {
    expect(config).toMatch(/const MOBILE_SPECS = \[[^\]]*\.\.\.MOBILE_ONLY_SPECS[^\]]*\];/);
    expect(projectBlock("mobile-chromium")).toContain("testMatch: MOBILE_SPECS");
    expect(config).toMatch(/name: 'mobile-webkit',\s*testMatch: MOBILE_SPECS/);
  });

  test.each(["chromium", "chromium-dom"])("MS3: the %s desktop project ignores MOBILE_ONLY_SPECS", (project) => {
    expect(projectBlock(project)).toMatch(/testIgnore: \[[^\]]*\.\.\.MOBILE_ONLY_SPECS/);
  });

  test("MS4: every other *-mobile.spec.ts on disk is in MOBILE_SPECS or the legacy allow-list", () => {
    const mobileSpecs = [...arrayLiteral("MOBILE_SPECS").map(globToRegExp), ...mobileOnly.map(globToRegExp)];
    const unregistered = fs
      .readdirSync(e2eDir)
      .filter((f) => f.endsWith("-mobile.spec.ts"))
      .filter((f) => !LEGACY_MOBILE_NAMED.includes(f))
      .filter((f) => !mobileSpecs.some((re) => re.test(f)));
    expect(unregistered).toEqual([]);
  });

  test("MS5: the legacy allow-list names only files that still exist", () => {
    for (const f of LEGACY_MOBILE_NAMED) expect(fs.existsSync(path.join(e2eDir, f))).toBe(true);
  });
});
