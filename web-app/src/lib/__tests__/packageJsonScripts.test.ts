import fs from "fs";
import path from "path";

const packageJson = JSON.parse(
  fs.readFileSync(path.join(__dirname, "../../../package.json"), "utf-8"),
);

describe("package.json scripts", () => {
  it("dev_should_containPortEnvVarFallback_When_readFromPackageJson", () => {
    expect(packageJson.scripts.dev).toContain("${PORT:-3001}");
  });

  it("start_should_containPortEnvVarFallback_When_readFromPackageJson", () => {
    expect(packageJson.scripts.start).toContain("${PORT:-3001}");
  });

  it("dev_should_containHostnameLocalhost_When_readFromPackageJson", () => {
    expect(packageJson.scripts.dev).toContain("--hostname localhost");
  });
});
