import { addRecentShellCommand, getRecentShellCommands } from "./recentShellCommands";

describe("recentShellCommands", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("getRecentShellCommands_should_returnEmpty_When_nothingStored", () => {
    expect(getRecentShellCommands()).toEqual([]);
  });

  it("addRecentShellCommand_should_persistCommand_When_called", () => {
    addRecentShellCommand("npm run dev");
    expect(getRecentShellCommands()).toEqual(["npm run dev"]);
  });

  it("addRecentShellCommand_should_ignoreBlank_When_whitespaceOnly", () => {
    addRecentShellCommand("   ");
    expect(getRecentShellCommands()).toEqual([]);
  });

  it("addRecentShellCommand_should_moveDuplicateToFront_When_reAdded", () => {
    addRecentShellCommand("npm run dev");
    addRecentShellCommand("go test ./...");
    addRecentShellCommand("npm run dev");
    expect(getRecentShellCommands()).toEqual(["npm run dev", "go test ./..."]);
  });

  it("addRecentShellCommand_should_capAtEightEntries_When_moreAdded", () => {
    for (let i = 0; i < 10; i++) {
      addRecentShellCommand(`command-${i}`);
    }
    const result = getRecentShellCommands();
    expect(result).toHaveLength(8);
    expect(result[0]).toBe("command-9");
  });
});
