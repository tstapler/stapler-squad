import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ImportSessionsContainer } from "./ImportSessionsContainer";
import {
  ImportStatus,
  ImportSourceKind,
  type ExternalSessionCandidateRef,
} from "@/gen/session/v1/import_pb";

jest.mock("./ImportSessionsContainer.css", () =>
  new Proxy({}, { get: (_target, key) => (typeof key === "string" ? key : "") })
);

const mockCommitImport = jest.fn();

jest.mock("@/lib/hooks/useImportSessionService", () => ({
  useImportSessionService: () => ({
    commitImport: mockCommitImport,
  }),
}));

// Stub ImportExternalSessionsPanel with a button that hands the container
// multiple candidates at once, the way a real "Import selected" bulk action
// does -- this is what Finding #4 exercises.
jest.mock("./ImportExternalSessionsPanel", () => ({
  ImportExternalSessionsPanel: ({
    onImport,
  }: {
    onImport?: (candidates: ExternalSessionCandidateRef[]) => void;
  }) => (
    <button
      data-testid="import-selected"
      onClick={() => onImport?.([candidate("a"), candidate("b"), candidate("c")])}
    >
      Import selected
    </button>
  ),
}));

// Stub ImportPreviewDialog so tests can directly trigger onConfirm/onCancel
// without exercising the real preview-fetch flow (covered separately in
// ImportPreviewDialog.test.tsx).
jest.mock("./ImportPreviewDialog", () => ({
  ImportPreviewDialog: ({
    candidate,
    onConfirm,
    onCancel,
  }: {
    candidate: ExternalSessionCandidateRef;
    onConfirm: (args: unknown) => void;
    onCancel: () => void;
  }) => (
    <div data-testid="preview-dialog">
      <span data-testid="preview-candidate-path">{candidate.path}</span>
      <button
        data-testid="confirm-preview"
        onClick={() =>
          onConfirm({
            preview: {
              correlation: { kind: 0, uuid: "", confidence: 0, candidates: [] },
              program: candidate.program,
              pidIdentity: undefined,
            },
            disambiguationChoice: undefined,
          })
        }
      >
        Confirm
      </button>
      <button data-testid="cancel-preview" onClick={onCancel}>
        Cancel
      </button>
    </div>
  ),
}));

jest.mock("./ConfirmKillDialog", () => ({
  ConfirmKillDialog: ({ onClose }: { onClose: () => void }) => (
    <div data-testid="kill-dialog">
      <button data-testid="close-kill-dialog" onClick={onClose}>
        Close
      </button>
    </div>
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

describe("ImportSessionsContainer bulk import queue", () => {
  beforeEach(() => {
    mockCommitImport.mockReset();
  });

  it("previews every selected candidate in turn, not just the first, when each is confirmed", async () => {
    mockCommitImport.mockResolvedValue({
      status: ImportStatus.COMMITTED,
      instanceId: "inst",
      error: "",
      pidIdentity: undefined,
    });

    render(<ImportSessionsContainer />);

    fireEvent.click(screen.getByTestId("import-selected"));

    expect(screen.getByTestId("preview-candidate-path")).toHaveTextContent(
      "/home/user/project-a"
    );

    fireEvent.click(screen.getByTestId("confirm-preview"));
    await waitFor(() =>
      expect(screen.getByTestId("preview-candidate-path")).toHaveTextContent(
        "/home/user/project-b"
      )
    );

    fireEvent.click(screen.getByTestId("confirm-preview"));
    await waitFor(() =>
      expect(screen.getByTestId("preview-candidate-path")).toHaveTextContent(
        "/home/user/project-c"
      )
    );

    fireEvent.click(screen.getByTestId("confirm-preview"));
    await waitFor(() =>
      expect(screen.queryByTestId("preview-dialog")).not.toBeInTheDocument()
    );

    expect(mockCommitImport).toHaveBeenCalledTimes(3);
  });

  it("advances to the next queued candidate when the current preview is cancelled", async () => {
    render(<ImportSessionsContainer />);

    fireEvent.click(screen.getByTestId("import-selected"));
    expect(screen.getByTestId("preview-candidate-path")).toHaveTextContent(
      "/home/user/project-a"
    );

    fireEvent.click(screen.getByTestId("cancel-preview"));

    await waitFor(() =>
      expect(screen.getByTestId("preview-candidate-path")).toHaveTextContent(
        "/home/user/project-b"
      )
    );
  });

  it("advances to the next queued candidate once the kill dialog is closed", async () => {
    mockCommitImport.mockResolvedValue({
      status: ImportStatus.COMMITTED,
      instanceId: "inst",
      error: "",
      pidIdentity: { pid: 1000, createTimeMs: BigInt(1) },
    });

    render(<ImportSessionsContainer />);

    fireEvent.click(screen.getByTestId("import-selected"));
    fireEvent.click(screen.getByTestId("confirm-preview"));

    await screen.findByTestId("kill-dialog");
    expect(screen.queryByTestId("preview-dialog")).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId("close-kill-dialog"));

    await waitFor(() =>
      expect(screen.getByTestId("preview-candidate-path")).toHaveTextContent(
        "/home/user/project-b"
      )
    );
  });
});
