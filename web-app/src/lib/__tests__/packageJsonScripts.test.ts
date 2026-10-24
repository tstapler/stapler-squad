import fs from "fs";
import path from "path";

const packageJson = JSON.parse(
  fs.readFileSync(path.join(__dirname, "../../../package.json"), "utf-8"),
);

describe("package.json scripts", () => {
  it("dev_should_containPortEnvVarFallback_When_readFromPackageJson", () => {
    expect(packageJson.scripts["dev:next"]).toContain("${PORT:-3001}");
  });

  it("start_should_containPortEnvVarFallback_When_readFromPackageJson", () => {
    expect(packageJson.scripts["start:next"]).toContain("${PORT:-3001}");
  });

  it("dev_should_containHostnameLocalhost_When_readFromPackageJson", () => {
    expect(packageJson.scripts["dev:next"]).toContain("--hostname localhost");
  });

  it("dev_and_start_and_build_should_dispatchOnWebBuild_When_readFromPackageJson", () => {
    for (const name of ["dev", "start", "build"]) {
      expect(packageJson.scripts[name]).toContain('"$WEB_BUILD" = bun');
      expect(packageJson.scripts[name]).toContain(`${name}:next`);
    }
  });
});
