import fs from "fs";
import path from "path";

const packageJson = JSON.parse(
  fs.readFileSync(path.join(__dirname, "../../../package.json"), "utf-8"),
);

describe("package.json scripts", () => {
  it("dev_and_build_should_runTheBunPipeline_When_readFromPackageJson", () => {
    expect(packageJson.scripts["dev"]).toBe("bun build/dev.ts");
    expect(packageJson.scripts["build"]).toBe("bun build/build.ts");
  });

  it("scripts_should_notReferenceNext_When_readFromPackageJson", () => {
    for (const [name, command] of Object.entries<string>(packageJson.scripts)) {
      expect({ name, command }).not.toEqual({ name, command: expect.stringMatching(/\bnext\b|WEB_BUILD/) });
    }
  });
});
