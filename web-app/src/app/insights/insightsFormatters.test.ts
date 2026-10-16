import { pathBasename } from "./insightsFormatters";

describe("pathBasename", () => {
  it("returns the session title for a worktree path, not the opaque id", () => {
    expect(
      pathBasename("/home/tstapler//stapler/squad/workspaces/d685c4b1a423cca3/worktrees/backlog/custom/workflow/stages/continue/18d46f93fa0ad119"),
    ).toBe("backlog-custom-workflow-stages-continue");
  });

  it("ignores a subdirectory of the worktree after the id", () => {
    expect(pathBasename("/h/worktrees/stapler/squad/steering/18cf7ea81b059424/tests/e2e")).toBe("stapler-squad-steering");
  });

  it("falls back to the last path segment outside a worktree", () => {
    expect(pathBasename("/home/tstapler/code/github/com/tstapler/kibitzer")).toBe("kibitzer");
  });
});
