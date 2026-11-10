/**
 * The mobile input layer relies on three ghostty-web behaviours verified by greps of one installed build
 * (Story 0.1.1): its keyCode 229 early return, its blanket `beforeinput` preventDefault, and the listeners on
 * its own hidden textarea. A different version may change any of them, so this fails until a human re-runs them.
 */
import { readFileSync } from "node:fs";
import { join } from "node:path";

const VERIFIED_GHOSTTY_WEB_VERSION = "0.4.0";

function ghosttyPinProblem(installed: string, verified = VERIFIED_GHOSTTY_WEB_VERSION): string | null {
  if (installed === verified) return null;
  return (
    `ghostty-web is ${installed}, but the mobile input layer was verified against ${verified}. ` +
    "Re-run the Story 0.1.1 greps on dist/ghostty-web.js (keyCode === 229 early return, blanket beforeinput " +
    "preventDefault, own-textarea listeners), re-run the Playwright check of Task 5.1.1d with the real module, " +
    "then update VERIFIED_GHOSTTY_WEB_VERSION."
  );
}

const installedVersion = (): string =>
  JSON.parse(readFileSync(join(__dirname, "../../../node_modules/ghostty-web/package.json"), "utf8")).version;

describe("ghostty-web version pin", () => {
  it("pin_should_Pass_When_InstalledVersionIsTheVerifiedOne", () => {
    expect(ghosttyPinProblem(installedVersion())).toBeNull();
  });

  it("pin_should_FailWithGreppingInstruction_When_GhosttyWebVersionChanges", () => {
    const problem = ghosttyPinProblem("0.5.0");
    expect(problem).toContain("Story 0.1.1");
    expect(problem).toContain("Task 5.1.1d");
    expect(problem).toContain("0.5.0");
  });

  it("pin_should_MatchPackageJsonPin_When_DependencyDeclaredExactly", () => {
    const pkg = JSON.parse(readFileSync(join(__dirname, "../../../package.json"), "utf8"));
    expect(pkg.dependencies["ghostty-web"]).toBe(VERIFIED_GHOSTTY_WEB_VERSION);
  });
});
