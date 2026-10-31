// T-E2-33: the e2e suite boots its own server on a free port with a private data
// directory, so it can never reach the live :8543 instance or its state.
import fs from "fs";
import path from "path";

const E2E = path.join(process.cwd(), "..", "tests", "e2e");
const read = (rel: string) => fs.readFileSync(path.join(E2E, rel), "utf8");

describe("e2e server isolation", () => {
  const testServer = read("helpers/test-server.ts");

  it("picks an OS-assigned free port and passes it to the server through PORT", () => {
    expect(testServer).toMatch(/srv\.listen\(0,/);
    expect(testServer).toMatch(/this\.config\.port = await \(attempt === 1 \? this\.config\.allocatePort \?\? findFreePort : findFreePort\)\(\)/);
    expect(testServer).toMatch(/PORT: this\.config\.port\.toString\(\)/);
  });

  it("starts the binary in test mode with a private --test-dir under /tmp", () => {
    expect(testServer).toMatch(/'--test-mode'/);
    expect(testServer).toMatch(/'--test-dir', this\.config\.testDir/);
    expect(testServer).toMatch(/\/tmp\/stapler-squad-test-\$\{pid\}/);
  });

  it("never names the live service port in the boot path", () => {
    for (const file of ["helpers/test-server.ts", "global-setup.ts", "playwright.config.ts"]) {
      expect(read(file)).not.toMatch(/\b8543\b/);
    }
  });

  it("exports the dynamic URL and data dir to workers from global setup", () => {
    const setup = read("global-setup.ts");
    expect(setup).toMatch(/process\.env\.TEST_SERVER_URLS = JSON\.stringify\(urls\)/);
    expect(setup).toMatch(/process\.env\.TEST_SERVER_TESTDIRS = JSON\.stringify\(dirs\)/);
    expect(setup).toMatch(/process\.env\.TEST_SERVER_URL = urls\[0\]/);
    expect(setup).toMatch(/process\.env\.TEST_SERVER_TESTDIR = dirs\[0\]/);
  });

  it("keeps specs off the raw single-server env vars (they would all hit worker slot 0)", () => {
    const allowed = [
      "helpers/base-url.ts",
      "helpers/test-server.ts",
      "global-setup",
      "playwright.config.ts",
      "playwright.dev-mode.config.ts",
      "worker-server-isolation.spec.ts",
      ".dev-mode.spec.ts",
      "helpers/session-client.ts", // doc comment only
    ];
    const offenders: string[] = [];
    const walk = (dir: string) => {
      for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const rel = path.relative(E2E, path.join(dir, entry.name));
        if (entry.isDirectory()) {
          if (!["node_modules", "allure-results", "perf", "baseline", "scripts"].includes(entry.name)) walk(path.join(dir, entry.name));
        } else if (entry.name.endsWith(".ts") && !allowed.some((a) => rel.includes(a))) {
          if (/process\.env\.(TEST_SERVER_URL|BASE_URL|TEST_SERVER_TESTDIR)\b/.test(read(rel))) offenders.push(rel);
        }
      }
    };
    walk(E2E);
    expect(offenders).toEqual([]);
  });
});
