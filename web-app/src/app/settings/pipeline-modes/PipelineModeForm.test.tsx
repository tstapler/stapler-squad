/**
 * Tests for PipelineModeForm — Epic 3.3 (Story 3.3.2) of
 * project_plans/backlog-configurable-pipeline/implementation/plan.md.
 *
 * Covers:
 *  1. Create success: submitting calls createPipelineMode with the entered
 *     payload and invokes onSaved with the server's returned mode (the
 *     contract page.tsx relies on to update the list without a page reload).
 *  2. Validation error (CodeInvalidArgument) is displayed inline; onSaved is
 *     NOT called and the form stays mounted (no navigation/list-refresh).
 *  3. Delete-with-confirm flow: Delete shows a "Confirm delete?" button;
 *     confirming calls deletePipelineMode and invokes onDeleted.
 *  4. Edit mode: slug field is disabled/read-only.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ConnectError, Code } from "@connectrpc/connect";
import { PipelineModeForm } from "./PipelineModeForm";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import type { PipelineMode } from "@/lib/hooks/useBacklogService";

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: jest.fn(),
}));

const mockUseBacklogService = useBacklogService as jest.MockedFunction<typeof useBacklogService>;

const mockCreatePipelineMode = jest.fn();
const mockUpdatePipelineMode = jest.fn();
const mockDeletePipelineMode = jest.fn();

function makeMode(overrides: Partial<PipelineMode> = {}): PipelineMode {
  return {
    id: "mode-1",
    slug: "quick",
    name: "Quick Fix",
    description: "Fast pipeline",
    enabled: true,
    statusCommandTemplate: "",
    doneCommandTemplate: "",
    failCommandTemplate: "",
    reviewCommandTemplate: "",
    shipCommandTemplate: "",
    helpCommandTemplate: "",
    triagePromptTemplate: "",
    reviewPromptTemplate: "",
    initialPromptTemplate: "",
    contentHash: "hash",
    ...overrides,
  };
}

// The jest styleMock for `.css.ts` files wraps every export (including plain
// `style()` string exports) in a callable proxy function, which triggers a
// benign "Invalid value for prop className" React warning here (same
// pre-existing jest/vanilla-extract mock limitation documented in
// BacklogItemForm.test.tsx and RadioGroup.test.tsx).
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterAll(() => {
  jest.restoreAllMocks();
});

beforeEach(() => {
  jest.clearAllMocks();
  mockUseBacklogService.mockReturnValue({
    createPipelineMode: mockCreatePipelineMode,
    updatePipelineMode: mockUpdatePipelineMode,
    deletePipelineMode: mockDeletePipelineMode,
  } as unknown as ReturnType<typeof useBacklogService>);
});

describe("PipelineModeForm", () => {
  it("create success: submits payload and calls onSaved without navigating away", async () => {
    const created = makeMode({ id: "mode-new", slug: "quick", name: "Quick Fix" });
    mockCreatePipelineMode.mockResolvedValue(created);
    const onSaved = jest.fn();
    const onCancel = jest.fn();
    const onDeleted = jest.fn();

    render(<PipelineModeForm mode={null} onSaved={onSaved} onDeleted={onDeleted} onCancel={onCancel} />);

    fireEvent.change(screen.getByTestId("pipeline-mode-slug"), { target: { value: "quick" } });
    fireEvent.change(screen.getByTestId("pipeline-mode-name"), { target: { value: "Quick Fix" } });
    fireEvent.change(screen.getByTestId("pipeline-mode-field-triagePromptTemplate"), {
      target: { value: "Fix {{item_id}} fast." },
    });

    fireEvent.click(screen.getByTestId("pipeline-mode-submit"));

    await waitFor(() => expect(mockCreatePipelineMode).toHaveBeenCalledTimes(1));
    expect(mockCreatePipelineMode).toHaveBeenCalledWith(
      expect.objectContaining({
        slug: "quick",
        name: "Quick Fix",
        triagePromptTemplate: "Fix {{item_id}} fast.",
      })
    );
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith(created));
    expect(onCancel).not.toHaveBeenCalled();
  });

  it("validation error: CodeInvalidArgument is shown inline, onSaved is not called, form stays mounted", async () => {
    mockCreatePipelineMode.mockRejectedValue(
      new ConnectError("slug must be lowercase alphanumeric with dashes", Code.InvalidArgument)
    );
    const onSaved = jest.fn();

    render(<PipelineModeForm mode={null} onSaved={onSaved} onDeleted={jest.fn()} onCancel={jest.fn()} />);

    fireEvent.change(screen.getByTestId("pipeline-mode-slug"), { target: { value: "Quick Fix!" } });
    fireEvent.change(screen.getByTestId("pipeline-mode-name"), { target: { value: "Quick Fix" } });
    fireEvent.click(screen.getByTestId("pipeline-mode-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("pipeline-mode-error")).toHaveTextContent(
        "slug must be lowercase alphanumeric with dashes"
      );
    });
    expect(onSaved).not.toHaveBeenCalled();
    // Form is still mounted — no navigation/list-refresh occurred.
    expect(screen.getByTestId("pipeline-mode-form")).toBeInTheDocument();
  });

  it("delete-with-confirm: Delete reveals Confirm delete?, confirming calls deletePipelineMode and onDeleted", async () => {
    mockDeletePipelineMode.mockResolvedValue(true);
    const existing = makeMode();
    const onDeleted = jest.fn();

    render(<PipelineModeForm mode={existing} onSaved={jest.fn()} onDeleted={onDeleted} onCancel={jest.fn()} />);

    expect(screen.queryByTestId("pipeline-mode-confirm-delete")).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId("pipeline-mode-delete"));
    expect(screen.getByTestId("pipeline-mode-confirm-delete")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("pipeline-mode-confirm-delete"));

    await waitFor(() => expect(mockDeletePipelineMode).toHaveBeenCalledWith(existing.id));
    await waitFor(() => expect(onDeleted).toHaveBeenCalledWith(existing.id));
  });

  it("delete-with-confirm: 'Never mind' cancels without calling deletePipelineMode", () => {
    const existing = makeMode();
    render(<PipelineModeForm mode={existing} onSaved={jest.fn()} onDeleted={jest.fn()} onCancel={jest.fn()} />);

    fireEvent.click(screen.getByTestId("pipeline-mode-delete"));
    expect(screen.getByTestId("pipeline-mode-confirm-delete")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("pipeline-mode-cancel-delete"));
    expect(screen.queryByTestId("pipeline-mode-confirm-delete")).not.toBeInTheDocument();
    expect(mockDeletePipelineMode).not.toHaveBeenCalled();
  });

  it("edit mode: slug field is disabled (immutable after creation)", () => {
    const existing = makeMode();
    render(<PipelineModeForm mode={existing} onSaved={jest.fn()} onDeleted={jest.fn()} onCancel={jest.fn()} />);

    const slugInput = screen.getByTestId("pipeline-mode-slug") as HTMLInputElement;
    expect(slugInput).toBeDisabled();
    expect(slugInput.value).toBe("quick");
  });

  it("edit mode: submitting calls updatePipelineMode with the mode's id, not createPipelineMode", async () => {
    const existing = makeMode();
    mockUpdatePipelineMode.mockResolvedValue({ ...existing, name: "Renamed" });
    const onSaved = jest.fn();

    render(<PipelineModeForm mode={existing} onSaved={onSaved} onDeleted={jest.fn()} onCancel={jest.fn()} />);

    fireEvent.change(screen.getByTestId("pipeline-mode-name"), { target: { value: "Renamed" } });
    fireEvent.click(screen.getByTestId("pipeline-mode-submit"));

    await waitFor(() => expect(mockUpdatePipelineMode).toHaveBeenCalledWith(existing.id, expect.objectContaining({ name: "Renamed" })));
    expect(mockCreatePipelineMode).not.toHaveBeenCalled();
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
  });
});
