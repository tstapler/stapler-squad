/**
 * BacklogFileBrowserModal has no SessionVcsProvider ancestor (unlike FilesTab),
 * so it must fetch its own VCS status to populate git status badges on the
 * embedded FileTree — this was previously wired to nothing (AC4).
 */

import React, { createRef } from "react";
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

// useFocusTrap.ts's own Tab-wrap behavior has its own dedicated suite
// (useFocusTrap.test.tsx) run against the real hook — mocked here so this
// file only asserts what's actually this component's responsibility: that
// it wires the hook onto its dialog ref with the trap active. Real,
// end-to-end Tab-wrap proof (as far as this modal's own markup goes; see
// that test's own comment for FileTree's separate, out-of-scope focus
// quirk) lives in tests/e2e/accessibility.spec.ts's Playwright coverage.
const useFocusTrapSpy = jest.fn();
jest.mock("@/lib/hooks/useFocusTrap", () => ({ useFocusTrap: (...args: unknown[]) => useFocusTrapSpy(...args) }));

beforeEach(() => {
  jest.clearAllMocks();
  capturedFileTreeProps = null;
});

describe("BacklogFileBrowserModal", () => {
  it("activates the focus trap on its dialog ref, returning focus to the supplied trigger", async () => {
    mockUseSessionVcs.mockReturnValue({ status: null });
    const triggerRef = createRef<HTMLButtonElement>();

    render(<BacklogFileBrowserModal sessionId="s1" onClose={() => {}} triggerRef={triggerRef} />);

    await waitFor(() => expect(useFocusTrapSpy).toHaveBeenCalled());
    const [refArg, isActiveArg, triggerRefArg] = useFocusTrapSpy.mock.calls[0];
    expect(refArg.current).toBeInstanceOf(HTMLElement);
    expect(refArg.current?.getAttribute("role")).toBe("dialog");
    expect(isActiveArg).toBe(true);
    expect(triggerRefArg).toBe(triggerRef);
  });

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
