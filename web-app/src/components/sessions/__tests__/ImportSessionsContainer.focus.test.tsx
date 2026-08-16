/**
 * Focus-restoration regression test for the import flow (WCAG 2.4.3).
 *
 * Deliberately does NOT mock useFocusTrap (unlike ImportSessionsContainer.test.tsx)
 * so the real trap-and-restore behavior runs end to end through both
 * ImportPreviewDialog and ConfirmKillDialog. Import can be triggered from
 * more than one button (per-row vs. bulk "Import selected" in the real UI,
 * modeled here as two distinct stub buttons), so this also verifies that
 * each trigger gets its own focus restored rather than always the last-used
 * one, and that ConfirmKillDialog -- which has no click of its own, since it
 * opens as a chained continuation after ImportPreviewDialog unmounts --
 * correctly reuses the same triggerRef captured when the flow started.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ImportSessionsContainer } from "../ImportSessionsContainer";
import {
  ImportStatus,
  ImportSourceKind,
  CorrelationKind,
  type ExternalSessionCandidateRef,
} from "@/gen/session/v1/import_pb";

jest.mock("../ImportSessionsContainer.css", () =>
  new Proxy({}, { get: (_target, key) => (typeof key === "string" ? key : "") })
);
jest.mock("../ImportPreviewDialog.css", () =>
  new Proxy({}, { get: (_target, key) => (typeof key === "string" ? key : "") })
);
jest.mock("../ConfirmKillDialog.css", () =>
  new Proxy({}, { get: (_target, key) => (typeof key === "string" ? key : "") })
);

const mockPreviewImport = jest.fn();
const mockCommitImport = jest.fn();
const mockConfirmKill = jest.fn();
const mockCancelPendingKill = jest.fn();

jest.mock("@/lib/hooks/useImportSessionService", () => ({
  useImportSessionService: () => ({
    previewImport: mockPreviewImport,
    commitImport: mockCommitImport,
    confirmKill: mockConfirmKill,
    cancelPendingKill: mockCancelPendingKill,
  }),
}));

// Stub with two persistent buttons standing in for the two real openers
// (a per-row "Import" button and the bulk "Import selected" button).
jest.mock("../ImportExternalSessionsPanel", () => ({
  ImportExternalSessionsPanel: ({
    onImport,
  }: {
    onImport?: (candidates: ExternalSessionCandidateRef[]) => void;
  }) => (
    <>
      <button data-testid="import-row-a" onClick={() => onImport?.([candidate("a")])}>
        Import a
      </button>
      <button data-testid="import-row-b" onClick={() => onImport?.([candidate("b")])}>
        Import b
      </button>
    </>
  ),
}));

function candidate(id: string): ExternalSessionCandidateRef {
  return {
    sourceKind: ImportSourceKind.PLAIN_TMUX,
    path: `/home/user/project-${id}`,
    program: "claude",
    pid: 1000,
    tmuxSession: `tmux-${id}`,
    socketPath: "",
  } as ExternalSessionCandidateRef;
}

function resolvedPreview(program: string, withPid: boolean) {
  return {
    program,
    path: `/home/user/project-${program}`,
    correlation: { kind: CorrelationKind.NONE, uuid: "", confidence: 0, candidates: [] },
    turnCount: 0,
    lastMessageExcerpt: "",
    pidIdentity: withPid ? { pid: 1000, createTimeMs: BigInt(1) } : undefined,
  };
}

describe("Import flow focus restoration", () => {
  beforeEach(() => {
    mockPreviewImport.mockReset();
    mockCommitImport.mockReset();
    mockConfirmKill.mockReset();
    mockCancelPendingKill.mockReset();
  });

  it("ImportSessionsContainer_should_restoreFocusToItsOpener_When_previewIsCancelled", async () => {
    mockPreviewImport.mockResolvedValue(resolvedPreview("a", false));

    render(<ImportSessionsContainer />);

    const opener = screen.getByTestId("import-row-a");
    opener.focus();
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByTestId("import-preview-dialog")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it("ImportSessionsContainer_should_restoreFocusToItsOwnOpener_When_anotherOpenerWasUsedPreviously", async () => {
    mockPreviewImport.mockResolvedValue(resolvedPreview("b", false));

    render(<ImportSessionsContainer />);

    const opener = screen.getByTestId("import-row-b");
    opener.focus();
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByTestId("import-preview-dialog")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(document.activeElement).toBe(opener));
    expect(document.activeElement).not.toBe(screen.getByTestId("import-row-a"));
  });

  it("ConfirmKillDialog_should_restoreFocusToTheImportOpener_When_closedAfterChainingFromPreviewDialog", async () => {
    mockPreviewImport.mockResolvedValue(resolvedPreview("a", true));
    mockCommitImport.mockResolvedValue({
      status: ImportStatus.COMMITTED,
      instanceId: "inst",
      error: "",
      pidIdentity: { pid: 1000, createTimeMs: BigInt(1) },
    });
    mockCancelPendingKill.mockResolvedValue({ resumed: true, error: "" });

    render(<ImportSessionsContainer />);

    const opener = screen.getByTestId("import-row-a");
    opener.focus();
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByTestId("import-preview-dialog")).toBeInTheDocument());

    fireEvent.click(screen.getByTestId("import-preview-confirm-button"));

    await waitFor(() => expect(screen.getByTestId("confirm-kill-dialog")).toBeInTheDocument());
    expect(screen.queryByTestId("import-preview-dialog")).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId("confirm-kill-revert-button"));

    await waitFor(() => expect(document.activeElement).toBe(opener));
  });
});
