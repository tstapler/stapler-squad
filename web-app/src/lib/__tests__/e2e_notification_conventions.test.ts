// T-E2-28: the e2e-test-conventions skill's hard rules, applied to the notification specs.
import fs from "fs";
import path from "path";

const E2E = path.join(process.cwd(), "..", "tests", "e2e");
const NOTIFICATION_SPEC = /^(notification|toast-stack|hidden-session|background-activity).*\.spec\.ts$/;

const specs = fs.readdirSync(E2E).filter((f) => NOTIFICATION_SPEC.test(f));
const read = (file: string) => fs.readFileSync(path.join(E2E, file), "utf8");

describe("notification e2e specs follow the e2e conventions", () => {
  it("finds the notification specs", () => {
    expect(specs.length).toBeGreaterThanOrEqual(8);
  });

  it.each(specs)("%s starts with a // @feature header", (file) => {
    expect(read(file).split("\n")[0]).toMatch(/^\/\/ @feature [\w:.-]+(, [\w:.-]+)*$/);
  });

  it.each(specs)("%s never calls waitForTimeout", (file) => {
    expect(read(file)).not.toMatch(/waitForTimeout\(/);
  });

  it.each(specs)("%s uses no CSS class selectors", (file) => {
    const classSelector = /\.(?:locator|\$|\$\$)\(\s*['"`](?:[a-z0-9]*\.[A-Za-z_-]|\.[A-Za-z_-]|[^'"`]*\[class)/;
    expect(read(file)).not.toMatch(classSelector);
  });
});
