/**
 * BacklogFileBrowserModal has no SessionVcsProvider ancestor (unlike FilesTab),
 * so it must fetch its own VCS status to populate git status badges on the
 * embedded FileTree — this was previously wired to nothing (AC4).
 */

import React from "react";
import { render, waitFor } from "@testing-library/react";
import { BacklogFileBrowserModal } from "../BacklogFileBrowserModal";
import { FileStatus } from "@/gen/session/v1/types_pb";

let capturedFileTreeProps: Record<string, unknown> | null = null;
jest.mock("@/components/sessions/FileTree", () => ({
  FileTree: (props: Record<string, unknown>) => {
    capturedFileTreeProps = props;
    return <div data-testid="mock-file-tree" />;
  },
}));

jest.mock("@/components/sessions/FileContentViewer", () => ({
  FileContentViewer: () => <div data-testid="mock-file-content-viewer" />,
}));

const mockUseSessionVcs = jest.fn();
jest.mock("@/lib/hooks/useSessionVcs", () => ({
  useSessionVcs: (...args: unknown[]) => mockUseSessionVcs(...args),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
}));

jest.mock("@/lib/hooks/useFocusTrap", () => ({ useFocusTrap: () => undefined }));

beforeEach(() => {
  jest.clearAllMocks();
  capturedFileTreeProps = null;
});

describe("BacklogFileBrowserModal", () => {
  it("passes a gitStatusMap built from useSessionVcs status into FileTree", async () => {
    mockUseSessionVcs.mockReturnValue({
      status: {
        stagedFiles: [],
        unstagedFiles: [{ path: "src/foo.go", status: FileStatus.MODIFIED, isStaged: false, oldPath: "", additions: 0, deletions: 0 }],
        untrackedFiles: [{ path: "src/new.go", status: FileStatus.UNTRACKED, isStaged: false, oldPath: "", additions: 0, deletions: 0 }],
      },
    });

    render(<BacklogFileBrowserModal sessionId="s1" onClose={() => {}} />);

    await waitFor(() => expect(capturedFileTreeProps).not.toBeNull());
    const gitStatusMap = capturedFileTreeProps!.gitStatusMap as Map<string, string>;
    expect(gitStatusMap.get("src/foo.go")).toBe("M");
    expect(gitStatusMap.get("src/new.go")).toBe("?");
  });

  it("passes an empty gitStatusMap while status is still loading", async () => {
    mockUseSessionVcs.mockReturnValue({ status: null });

    render(<BacklogFileBrowserModal sessionId="s1" onClose={() => {}} />);

    await waitFor(() => expect(capturedFileTreeProps).not.toBeNull());
    const gitStatusMap = capturedFileTreeProps!.gitStatusMap as Map<string, string>;
    expect(gitStatusMap.size).toBe(0);
  });
});
