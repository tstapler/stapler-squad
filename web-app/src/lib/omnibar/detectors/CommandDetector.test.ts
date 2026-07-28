import { CommandDetector, parseShellArgs } from "./CommandDetector";
import { InputType } from "../types";

describe("parseShellArgs", () => {
  it("parseShellArgs_should_returnEmpty_When_noArgs", () => {
    expect(parseShellArgs(undefined)).toEqual({});
    expect(parseShellArgs("")).toEqual({});
    expect(parseShellArgs("   ")).toEqual({});
  });

  it("parseShellArgs_should_returnDirOnly_When_noDashDash", () => {
    expect(parseShellArgs("~/code/my-repo")).toEqual({ dir: "~/code/my-repo" });
  });

  it("parseShellArgs_should_returnCommandOnly_When_dashDashAtStart", () => {
    expect(parseShellArgs("-- npm run dev")).toEqual({ command: "npm run dev" });
  });

  it("parseShellArgs_should_returnDirAndCommand_When_both", () => {
    expect(parseShellArgs("~/code/my-repo -- npm run dev")).toEqual({
      dir: "~/code/my-repo",
      command: "npm run dev",
    });
  });
});

describe("CommandDetector", () => {
  const detector = new CommandDetector();

  it("CommandDetector_should_returnNull_When_notPrefixedWithGt", () => {
    expect(detector.detect("shell ~/code")).toBeNull();
  });

  it("CommandDetector_should_detectBareShell_When_noArgs", () => {
    const result = detector.detect(">shell");
    expect(result?.type).toBe(InputType.SpawnShell);
    expect(result?.metadata).toEqual({ commandType: "spawn_shell", shellDir: undefined, shellCommand: undefined });
    expect(result?.suggestedName).toBe("Open terminal");
  });

  it("CommandDetector_should_detectShellWithDir_When_dirGiven", () => {
    const result = detector.detect(">shell ~/code/my-repo");
    expect(result?.type).toBe(InputType.SpawnShell);
    expect(result?.metadata).toEqual({
      commandType: "spawn_shell",
      shellDir: "~/code/my-repo",
      shellCommand: undefined,
    });
    expect(result?.suggestedName).toBe("Open terminal in ~/code/my-repo");
  });

  it("CommandDetector_should_detectShellWithCommand_When_dashDashGiven", () => {
    const result = detector.detect(">shell -- npm run dev");
    expect(result?.type).toBe(InputType.SpawnShell);
    expect(result?.metadata).toEqual({
      commandType: "spawn_shell",
      shellDir: undefined,
      shellCommand: "npm run dev",
    });
    expect(result?.suggestedName).toBe('Run "npm run dev"');
  });

  it("CommandDetector_should_detectShellWithDirAndCommand_When_bothGiven", () => {
    const result = detector.detect(">shell ~/code/my-repo -- npm run dev");
    expect(result?.type).toBe(InputType.SpawnShell);
    expect(result?.metadata).toEqual({
      commandType: "spawn_shell",
      shellDir: "~/code/my-repo",
      shellCommand: "npm run dev",
    });
    expect(result?.suggestedName).toBe('Run "npm run dev" in ~/code/my-repo');
  });

  it("CommandDetector_should_detectTheme_When_themeCommand", () => {
    const result = detector.detect(">theme matrix");
    expect(result?.type).toBe(InputType.Command);
    expect(result?.metadata).toEqual({ commandType: "theme", commandArg: "matrix" });
  });

  it("CommandDetector_should_returnUnknown_When_unrecognizedCommand", () => {
    const result = detector.detect(">bogus");
    expect(result?.type).toBe(InputType.Unknown);
  });
});
