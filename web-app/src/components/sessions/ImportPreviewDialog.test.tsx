import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ImportPreviewDialog } from "./ImportPreviewDialog";
import {
  CorrelationKind,
  CorrelationConfidence,
  ImportSourceKind,
  type ExternalSessionCandidateRef,
  type PreviewImportExternalSessionResponse,
} from "@/gen/session/v1/import_pb";

jest.mock("./ImportPreviewDialog.css", () =>
  new Proxy(
    {},
    {
      get: (_target, key) => (typeof key === "string" ? key : ""),
    }
  )
);

jest.mock("@/lib/hooks/useFocusTrap", () => ({
  useFocusTrap: jest.fn(),
}));

const mockPreviewImport = jest.fn();

jest.mock("@/lib/hooks/useImportSessionService", () => ({
  useImportSessionService: () => ({
    previewImport: mockPreviewImport,
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

function resolvedPreview(
  overrides: Partial<PreviewImportExternalSessionResponse> = {}
): PreviewImportExternalSessionResponse {
  return {
    program: "claude",
    path: "/home/user/project",
    correlation: {
      kind: CorrelationKind.RESOLVED,
      uuid: "uuid-1",
      confidence: CorrelationConfidence.PID_EXACT,
      candidates: [],
    },
    turnCount: 12,
    lastMessageExcerpt: "Let's fix the bug in the parser.",
    pidIdentity: { pid: 4321, createTimeMs: BigInt(1000) },
    ...overrides,
  } as PreviewImportExternalSessionResponse;
}

function ambiguousPreview(): PreviewImportExternalSessionResponse {
  return {
    program: "claude",
    path: "/home/user/project",
    correlation: {
      kind: CorrelationKind.AMBIGUOUS,
      uuid: "",
      confidence: CorrelationConfidence.PATH_HEURISTIC,
      candidates: [
        {
          conversationUuid: "uuid-a",
          historyFilePath: "/history/a.jsonl",
          projectDir: "/home/user/project-a",
        },
        {
          conversationUuid: "uuid-b",
          historyFilePath: "/history/b.jsonl",
          projectDir: "/home/user/project-b",
        },
      ],
    },
    turnCount: 0,
    lastMessageExcerpt: "",
    pidIdentity: { pid: 4321, createTimeMs: BigInt(1000) },
  } as PreviewImportExternalSessionResponse;
}

function notFoundPreview(): PreviewImportExternalSessionResponse {
  return {
    program: "claude",
    path: "/home/user/project",
    correlation: {
      kind: CorrelationKind.NOT_FOUND,
      uuid: "",
      confidence: CorrelationConfidence.NONE,
      candidates: [],
    },
    turnCount: 0,
    lastMessageExcerpt: "",
    pidIdentity: undefined,
  } as unknown as PreviewImportExternalSessionResponse;
}

describe("ImportPreviewDialog", () => {
  beforeEach(() => {
    mockPreviewImport.mockReset();
  });

  it("shows a loading state while the preview request is in flight", () => {
    mockPreviewImport.mockReturnValue(new Promise(() => {}));

    render(
      <ImportPreviewDialog
        candidate={candidate()}
        onConfirm={jest.fn()}
        onCancel={jest.fn()}
      />
    );

    expect(screen.getByText(/Looking up session history/i)).toBeInTheDocument();
  });

  it("renders RESOLVED correlation with turn count and excerpt", async () => {
    mockPreviewImport.mockResolvedValue(resolvedPreview());

    render(
      <ImportPreviewDialog
        candidate={candidate()}
        onConfirm={jest.fn()}
        onCancel={jest.fn()}
      />
    );

    expect(await screen.findByText(/History matched/i)).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(
      screen.getByText("Let's fix the bug in the parser.")
    ).toBeInTheDocument();
  });

  it("shows the SIGSTOP banner when pidIdentity is present", async () => {
    mockPreviewImport.mockResolvedValue(resolvedPreview());

    render(
      <ImportPreviewDialog
        candidate={candidate()}
        onConfirm={jest.fn()}
        onCancel={jest.fn()}
      />
    );

    expect(await screen.findByText(/suspend \(SIGSTOP\)/i)).toBeInTheDocument();
    expect(screen.getByText(/PID 4321/)).toBeInTheDocument();
  });

  it("renders AMBIGUOUS candidates and defaults selection to the first one", async () => {
    mockPreviewImport.mockResolvedValue(ambiguousPreview());
    const onConfirm = jest.fn();

    render(
      <ImportPreviewDialog
        candidate={candidate()}
        onConfirm={onConfirm}
        onCancel={jest.fn()}
      />
    );

    expect(await screen.findByText(/Multiple histories found/i)).toBeInTheDocument();
    expect(screen.getByText("/home/user/project-a")).toBeInTheDocument();
    expect(screen.getByText("/home/user/project-b")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Import Session" }));

    await waitFor(() => {
      expect(onConfirm).toHaveBeenCalledWith(
        expect.objectContaining({ disambiguationChoice: "uuid-a" })
      );
    });
  });

  it("allows switching the AMBIGUOUS selection before confirming", async () => {
    mockPreviewImport.mockResolvedValue(ambiguousPreview());
    const onConfirm = jest.fn();

    render(
      <ImportPreviewDialog
        candidate={candidate()}
        onConfirm={onConfirm}
        onCancel={jest.fn()}
      />
    );

    await screen.findByText(/Multiple histories found/i);

    fireEvent.click(screen.getByText("uuid-b"));
    fireEvent.click(screen.getByRole("button", { name: "Import Session" }));

    await waitFor(() => {
      expect(onConfirm).toHaveBeenCalledWith(
        expect.objectContaining({ disambiguationChoice: "uuid-b" })
      );
    });
  });

  it("renders NOT_FOUND correlation without a candidate list or excerpt", async () => {
    mockPreviewImport.mockResolvedValue(notFoundPreview());

    render(
      <ImportPreviewDialog
        candidate={candidate()}
        onConfirm={jest.fn()}
        onCancel={jest.fn()}
      />
    );

    expect(await screen.findByText(/No history matched/i)).toBeInTheDocument();
    expect(screen.queryByRole("radiogroup")).not.toBeInTheDocument();
    expect(screen.queryByText(/suspend \(SIGSTOP\)/i)).not.toBeInTheDocument();
  });

  it("shows an error state when preview fails", async () => {
    mockPreviewImport.mockResolvedValue(null);

    render(
      <ImportPreviewDialog
        candidate={candidate()}
        onConfirm={jest.fn()}
        onCancel={jest.fn()}
      />
    );

    expect(
      await screen.findByText(/Failed to preview this session/i)
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Import Session" })).toBeDisabled();
  });

  it("calls onConfirm with the full preview response for RESOLVED sessions", async () => {
    const preview = resolvedPreview();
    mockPreviewImport.mockResolvedValue(preview);
    const onConfirm = jest.fn();

    render(
      <ImportPreviewDialog
        candidate={candidate()}
        onConfirm={onConfirm}
        onCancel={jest.fn()}
      />
    );

    await screen.findByText(/History matched/i);
    fireEvent.click(screen.getByRole("button", { name: "Import Session" }));

    await waitFor(() => {
      expect(onConfirm).toHaveBeenCalledWith({
        preview,
        disambiguationChoice: undefined,
      });
    });
  });

  it("invokes onCancel when the Cancel button is clicked", async () => {
    mockPreviewImport.mockResolvedValue(resolvedPreview());
    const onCancel = jest.fn();

    render(
      <ImportPreviewDialog
        candidate={candidate()}
        onConfirm={jest.fn()}
        onCancel={onCancel}
      />
    );

    await screen.findByText(/History matched/i);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(onCancel).toHaveBeenCalledTimes(1);
  });
});
