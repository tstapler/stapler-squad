/**
 * Focus-restoration regression test for the import flow (WCAG 2.4.3):
 * ImportPreviewDialog, and the chained ConfirmKillDialog that follows it.
 *
 * Deliberately does NOT mock useFocusTrap so the real trap-and-restore
 * behavior runs end to end (unlike ImportPreviewDialog.test.tsx /
 * ConfirmKillDialog.test.tsx, which stub it out).
 *
 * ConfirmKillDialog has no live DOM trigger of its own — per
 * ImportSessionsContainer.tsx and the doc comment on ConfirmKillDialog's
 * `triggerRef` prop, it opens as a chained continuation after
 * ImportPreviewDialog unmounts, reusing the *same* triggerRef object that
 * captured the original "Import" button click. This harness mirrors that
 * exact chain (one triggerRef, shared across both dialogs, never reassigned
 * in between) to prove focus lands back on the original opener once the
 * whole chain closes — not just that each dialog traps focus in isolation.
 */

import React, { useRef } from "react";
import type { MouseEvent } from "react";
import { render, fireEvent, waitFor, screen } from "@testing-library/react";
import { ImportPreviewDialog } from "../ImportPreviewDialog";
import { ConfirmKillDialog } from "../ConfirmKillDialog";
import {
  CorrelationKind,
  CorrelationConfidence,
  ImportSourceKind,
  type ExternalSessionCandidateRef,
  type PreviewImportExternalSessionResponse,
} from "@/gen/session/v1/import_pb";

jest.mock("../ImportPreviewDialog.css", () =>
  new Proxy({}, { get: (_t, key) => (typeof key === "string" ? key : "") })
);
jest.mock("../ConfirmKillDialog.css", () =>
  new Proxy({}, { get: (_t, key) => (typeof key === "string" ? key : "") })
);

const mockPreviewImport = jest.fn();
const mockCancelPendingKill = jest.fn();

jest.mock("@/lib/hooks/useImportSessionService", () => ({
  useImportSessionService: () => ({
    previewImport: mockPreviewImport,
    confirmKill: jest.fn(),
    cancelPendingKill: mockCancelPendingKill,
  }),
}));

function candidate(): ExternalSessionCandidateRef {
  return {
    sourceKind: ImportSourceKind.PLAIN_TMUX,
    path: "/home/user/project",
    program: "claude",
    pid: 4321,
    tmuxSession: "tmux-1",
    socketPath: "",
  } as ExternalSessionCandidateRef;
}

function resolvedPreview(): PreviewImportExternalSessionResponse {
  return {
    program: "claude",
    path: "/home/user/project",
    correlation: {
      kind: CorrelationKind.RESOLVED,
      confidence: CorrelationConfidence.PID_EXACT,
      candidates: [],
    },
  } as unknown as PreviewImportExternalSessionResponse;
}

type Stage = "closed" | "preview" | "kill";

function Harness() {
  const [stage, setStage] = React.useState<Stage>("closed");
  const importTriggerRef = useRef<HTMLElement | null>(null);

  const handleImportClick = (event: MouseEvent<HTMLButtonElement>) => {
    // Mirrors ImportSessionsContainer.handleImport: capture once, reuse
    // across the whole chained flow.
    importTriggerRef.current = event.currentTarget;
    setStage("preview");
  };

  return (
    <>
      <button data-testid="import-button" onClick={handleImportClick}>
        Import
      </button>
      {stage === "preview" && (
        <ImportPreviewDialog
          candidate={candidate()}
          onConfirm={() => setStage("kill")}
          onCancel={() => setStage("closed")}
          triggerRef={importTriggerRef}
        />
      )}
      {stage === "kill" && (
        <ConfirmKillDialog
          instanceId="instance-1"
          program="claude"
          onStatusChange={() => {}}
          onClose={() => setStage("closed")}
          triggerRef={importTriggerRef}
        />
      )}
    </>
  );
}

describe("Import flow focus restoration", () => {
  beforeEach(() => {
    mockPreviewImport.mockReset().mockResolvedValue(resolvedPreview());
    mockCancelPendingKill.mockReset().mockResolvedValue({ resumed: true });
  });

  it("ImportPreviewDialog_should_restoreFocusToImportButton_When_cancelled", async () => {
    render(<Harness />);
    const opener = screen.getByTestId("import-button");
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByTestId("import-preview-dialog")).not.toBeNull());
    fireEvent.click(screen.getByText("Cancel"));
    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it("ConfirmKillDialog_should_restoreFocusToOriginalImportButton_When_chainedFromPreviewDialogAndClosed", async () => {
    render(<Harness />);
    const opener = screen.getByTestId("import-button");
    fireEvent.click(opener);
    await waitFor(() => expect(screen.getByTestId("import-preview-dialog")).not.toBeNull());

    fireEvent.click(screen.getByTestId("import-preview-confirm-button"));
    await waitFor(() => expect(screen.getByTestId("confirm-kill-dialog")).not.toBeNull());

    fireEvent.click(screen.getByTestId("confirm-kill-revert-button"));
    await waitFor(() => expect(screen.queryByTestId("confirm-kill-dialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(opener));
  });
});
