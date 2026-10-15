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
    expect(testServer).toMatch(/this\.config\.port = await findFreePort\(\)/);
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
    expect(setup).toMatch(/process\.env\.TEST_SERVER_URL = getGlobalTestServer\(\)\.getBaseUrl\(\)/);
    expect(setup).toMatch(/process\.env\.TEST_SERVER_TESTDIR = getGlobalTestServer\(\)\.getTestDir\(\)/);
  });
});
