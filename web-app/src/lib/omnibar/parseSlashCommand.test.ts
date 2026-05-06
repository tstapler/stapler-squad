import { parseSlashCommand, KNOWN_SLASH_COMMANDS } from "./parseSlashCommand";

describe("parseSlashCommand", () => {
  it("returns null for plain text", () => {
    expect(parseSlashCommand("my feature")).toBeNull();
  });

  it("returns null for unknown slash commands", () => {
    expect(parseSlashCommand("/unknown")).toBeNull();
  });

  it("returns null for absolute paths (should fall through to LocalPathDetector)", () => {
    expect(parseSlashCommand("/opt/homebrew")).toBeNull();
    expect(parseSlashCommand("/Users/foo/project")).toBeNull();
  });

  it("parses /oneoff → one_off with empty remainder", () => {
    const result = parseSlashCommand("/oneoff");
    expect(result?.sessionType).toBe("one_off");
    expect(result?.remainder).toBe("");
  });

  it("parses /worktree → new_worktree with remainder", () => {
    const result = parseSlashCommand("/worktree my feature");
    expect(result?.sessionType).toBe("new_worktree");
    expect(result?.remainder).toBe("my feature");
  });

  it("parses /dir → directory", () => {
    const result = parseSlashCommand("/dir");
    expect(result?.sessionType).toBe("directory");
  });

  it("parses /existing → existing_worktree", () => {
    const result = parseSlashCommand("/existing");
    expect(result?.sessionType).toBe("existing_worktree");
  });

  it("parses /project → new_project", () => {
    const result = parseSlashCommand("/project");
    expect(result?.sessionType).toBe("new_project");
  });

  it("is case-insensitive", () => {
    expect(parseSlashCommand("/ONEOFF")?.sessionType).toBe("one_off");
    expect(parseSlashCommand("/Worktree")?.sessionType).toBe("new_worktree");
  });

  it("trims whitespace from remainder", () => {
    const result = parseSlashCommand("/oneoff   my session   ");
    expect(result?.remainder).toBe("my session");
  });

  it("KNOWN_SLASH_COMMANDS covers all session types", () => {
    const values = Object.values(KNOWN_SLASH_COMMANDS);
    expect(values).toContain("one_off");
    expect(values).toContain("new_worktree");
    expect(values).toContain("directory");
    expect(values).toContain("existing_worktree");
    expect(values).toContain("new_project");
  });
});
