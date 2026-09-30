import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { WorkspaceSwitcher } from "./WorkspaceSwitcher";
import { truncateWorkspacePath } from "@/lib/utils/truncateWorkspacePath";
import type { DatabaseInfo } from "@/gen/session/v1/types_pb";

const mockUseDatabases = jest.fn();

jest.mock("@/lib/hooks/useDatabase", () => ({
  useDatabases: () => mockUseDatabases(),
}));

function makeDb(overrides: Partial<DatabaseInfo> & { workspaceId: string }): DatabaseInfo {
  return {
    type: "workspace",
    cwd: "",
    name: overrides.workspaceId,
    isCurrent: false,
    sessionCount: 0,
    ...overrides,
    configDir: overrides.configDir ?? `/config/${overrides.workspaceId}`,
  } as DatabaseInfo;
}

function setUseDatabases(databases: DatabaseInfo[]) {
  mockUseDatabases.mockReturnValue({
    databases,
    currentId: databases.find((d) => d.isCurrent)?.workspaceId ?? "",
    switching: false,
    merging: false,
    error: null,
    switchDatabase: jest.fn(),
    mergeDatabase: jest.fn(),
  });
}

function openDropdown() {
  fireEvent.click(screen.getByTitle("Switch workspace"));
}

describe("WorkspaceSwitcher — workspace path truncation", () => {
  afterEach(() => {
    mockUseDatabases.mockReset();
  });

  it("WorkspaceSwitcher_should_TruncatePathAt36Chars_When_PathExceedsThreshold", () => {
    const longPath =
      "/Users/tstapler/code/github.com/some-org/a-very-long-repository-name-for-testing";
    setUseDatabases([
      makeDb({ workspaceId: "current", name: "current-ws", isCurrent: true }),
      makeDb({ workspaceId: "other", name: "other-ws", cwd: longPath }),
    ]);

    render(<WorkspaceSwitcher />);
    openDropdown();

    const expected = truncateWorkspacePath(longPath, 36);
    expect(screen.getByText(expected)).toBeInTheDocument();
  });

  it("WorkspaceSwitcher_should_RenderPathUnchanged_When_PathUnderThreshold", () => {
    const shortPath = "/tmp/short";
    setUseDatabases([
      makeDb({ workspaceId: "current", name: "current-ws", isCurrent: true }),
      makeDb({ workspaceId: "other", name: "other-ws", cwd: shortPath }),
    ]);

    render(<WorkspaceSwitcher />);
    openDropdown();

    expect(truncateWorkspacePath(shortPath, 36)).toBe(shortPath);
    expect(screen.getByText(shortPath)).toBeInTheDocument();
  });
});
