/**
 * Tests for LivenessSection — the settings UI for the 5 LivenessDefinition
 * CRUD RPCs (Epic 1.3 of backlog-custom-workflow-stages), deferred until this
 * project. Covers the 4 acceptance criteria:
 *  0. Viewing a stage's rows, including a per-pipeline-mode override and its
 *     resolution-chain hint.
 *  1. Create, edit, and delete without hand-rolling RPC calls, plus
 *     client-side validation (kind-shape and positivity — neither enforced
 *     server-side, see pitfalls.md §3).
 *  3. A pre-existing mode-specific row (e.g. an sdd-mode override) displays
 *     and remains editable.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { LivenessSection } from "./LivenessSection";
import { useLivenessDefinitions } from "@/lib/hooks/useLivenessDefinitions";
import type { LivenessDefinition } from "@/lib/hooks/useLivenessDefinitions";
import type { PipelineMode } from "@/lib/hooks/useBacklogService";

jest.mock("@/lib/hooks/useLivenessDefinitions", () => ({
  useLivenessDefinitions: jest.fn(),
}));

const mockUseLivenessDefinitions = useLivenessDefinitions as jest.MockedFunction<typeof useLivenessDefinitions>;

const mockList = jest.fn();
const mockCreate = jest.fn();
const mockUpdate = jest.fn();
const mockDelete = jest.fn();

function makePipelineMode(overrides: Partial<PipelineMode> & Pick<PipelineMode, "id" | "slug" | "name">): PipelineMode {
  return {
    description: "",
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

const SDD_MODE = makePipelineMode({ id: "p1", slug: "sdd", name: "SDD Mode" });

function makeDefinition(overrides: Partial<LivenessDefinition> & Pick<LivenessDefinition, "id" | "stageSlug" | "kind">): LivenessDefinition {
  return {
    expectedDurationMs: 0,
    stalenessMarginMs: 0,
    maxNoProgressDurationMs: 0,
    cycleThreshold: 0,
    cycleLookbackMs: 0,
    enabled: true,
    ...overrides,
  };
}

beforeEach(() => {
  jest.clearAllMocks();
  mockUseLivenessDefinitions.mockReturnValue({
    listLivenessDefinitions: mockList,
    createLivenessDefinition: mockCreate,
    updateLivenessDefinition: mockUpdate,
    deleteLivenessDefinition: mockDelete,
  });
});

describe("LivenessSection", () => {
  it("AC0: renders the mode-less default row and a per-mode override row with a resolution hint", async () => {
    mockList.mockResolvedValue([
      makeDefinition({ id: "d1", stageSlug: "idea", kind: "duration_budget", expectedDurationMs: 3 * 3600000, stalenessMarginMs: 900000 }),
      makeDefinition({ id: "d2", stageSlug: "idea", pipelineMode: "sdd", kind: "duration_budget", expectedDurationMs: 45 * 60000 }),
    ]);

    render(<LivenessSection stageSlug="idea" pipelineModeOptions={[SDD_MODE]} />);

    expect(await screen.findByTestId("liveness-row-default")).toHaveTextContent("All modes (default)");
    expect(screen.getByTestId("liveness-row-sdd")).toHaveTextContent("Mode: sdd");
    expect(screen.getByTestId("liveness-row-sdd")).toHaveTextContent('Overrides mode "sdd" only');
    expect(screen.getByTestId("liveness-row-sdd")).toHaveTextContent("fall through to the stage default above");
    expect(screen.getByTestId("liveness-fallback-note")).toHaveTextContent("Modes without their own override use the stage-wide default row above.");
  });

  it("AC0: with no base row, surfaces the built-in-default fallback note instead of implying stage-wide coverage", async () => {
    mockList.mockResolvedValue([makeDefinition({ id: "d2", stageSlug: "idea", pipelineMode: "sdd", kind: "duration_budget", expectedDurationMs: 45 * 60000 })]);

    render(<LivenessSection stageSlug="idea" pipelineModeOptions={[SDD_MODE]} />);

    await screen.findByTestId("liveness-row-sdd");
    expect(screen.getByTestId("liveness-row-sdd")).toHaveTextContent("fall back to the built-in default");
    expect(screen.getByTestId("liveness-fallback-note")).toHaveTextContent("built-in default (duration budget: 3h expected + 15m margin)");
  });

  it("AC1: creates a new duration_budget override and round-trips it into the list", async () => {
    mockList.mockResolvedValue([]);
    const created = makeDefinition({ id: "new-1", stageSlug: "idea", kind: "duration_budget", expectedDurationMs: 60 * 60000, stalenessMarginMs: 10 * 60000 });
    mockCreate.mockResolvedValue(created);

    render(<LivenessSection stageSlug="idea" pipelineModeOptions={[]} />);
    await screen.findByText("No liveness overrides configured for this stage.");

    fireEvent.click(screen.getByTestId("liveness-add"));
    fireEvent.change(screen.getByTestId("liveness-editor-expected"), { target: { value: "60" } });
    fireEvent.change(screen.getByTestId("liveness-editor-margin"), { target: { value: "10" } });
    fireEvent.click(screen.getByTestId("liveness-editor-save"));

    await waitFor(() =>
      expect(mockCreate).toHaveBeenCalledWith(
        expect.objectContaining({ stageSlug: "idea", kind: "duration_budget", expectedDurationMs: 3600000, stalenessMarginMs: 600000 })
      )
    );
    expect(await screen.findByTestId("liveness-row-default")).toBeInTheDocument();
  });

  it("AC1: creates a new heartbeat override and round-trips it into the list", async () => {
    mockList.mockResolvedValue([]);
    const created = makeDefinition({ id: "new-2", stageSlug: "idea", kind: "heartbeat", maxNoProgressDurationMs: 120 * 60000 });
    mockCreate.mockResolvedValue(created);

    render(<LivenessSection stageSlug="idea" pipelineModeOptions={[]} />);
    await screen.findByText("No liveness overrides configured for this stage.");

    fireEvent.click(screen.getByTestId("liveness-add"));
    fireEvent.change(screen.getByTestId("liveness-editor-kind"), { target: { value: "heartbeat" } });
    fireEvent.change(screen.getByTestId("liveness-editor-no-progress"), { target: { value: "120" } });
    fireEvent.click(screen.getByTestId("liveness-editor-save"));

    await waitFor(() =>
      expect(mockCreate).toHaveBeenCalledWith(expect.objectContaining({ stageSlug: "idea", kind: "heartbeat", maxNoProgressDurationMs: 7200000 }))
    );
    expect(await screen.findByTestId("liveness-row-default")).toHaveTextContent("max no-progress 2h");
  });

  it("AC1: rejects a zero max no-progress duration for heartbeat client-side without calling the RPC", async () => {
    mockList.mockResolvedValue([]);
    render(<LivenessSection stageSlug="idea" pipelineModeOptions={[]} />);
    await screen.findByText("No liveness overrides configured for this stage.");

    fireEvent.click(screen.getByTestId("liveness-add"));
    fireEvent.change(screen.getByTestId("liveness-editor-kind"), { target: { value: "heartbeat" } });
    fireEvent.change(screen.getByTestId("liveness-editor-no-progress"), { target: { value: "0" } });
    fireEvent.click(screen.getByTestId("liveness-editor-save"));

    expect(await screen.findByTestId("liveness-editor-error")).toHaveTextContent("Max no-progress duration must be a positive number of minutes.");
    expect(mockCreate).not.toHaveBeenCalled();
  });

  it("AC1: creates a new cycle_frequency override and round-trips it into the list", async () => {
    mockList.mockResolvedValue([]);
    const created = makeDefinition({ id: "new-3", stageSlug: "idea", kind: "cycle_frequency", cycleThreshold: 3, cycleLookbackMs: 24 * 3600000 });
    mockCreate.mockResolvedValue(created);

    render(<LivenessSection stageSlug="idea" pipelineModeOptions={[]} />);
    await screen.findByText("No liveness overrides configured for this stage.");

    fireEvent.click(screen.getByTestId("liveness-add"));
    fireEvent.change(screen.getByTestId("liveness-editor-kind"), { target: { value: "cycle_frequency" } });
    fireEvent.change(screen.getByTestId("liveness-editor-threshold"), { target: { value: "3" } });
    fireEvent.change(screen.getByTestId("liveness-editor-lookback"), { target: { value: "1440" } });
    fireEvent.click(screen.getByTestId("liveness-editor-save"));

    await waitFor(() =>
      expect(mockCreate).toHaveBeenCalledWith(
        expect.objectContaining({ stageSlug: "idea", kind: "cycle_frequency", cycleThreshold: 3, cycleLookbackMs: 86400000 })
      )
    );
    expect(await screen.findByTestId("liveness-row-default")).toHaveTextContent("3 cycles / 24h lookback");
  });

  it("AC1: rejects a zero cycle threshold client-side without calling the RPC", async () => {
    mockList.mockResolvedValue([]);
    render(<LivenessSection stageSlug="idea" pipelineModeOptions={[]} />);
    await screen.findByText("No liveness overrides configured for this stage.");

    fireEvent.click(screen.getByTestId("liveness-add"));
    fireEvent.change(screen.getByTestId("liveness-editor-kind"), { target: { value: "cycle_frequency" } });
    fireEvent.change(screen.getByTestId("liveness-editor-threshold"), { target: { value: "0" } });
    fireEvent.change(screen.getByTestId("liveness-editor-lookback"), { target: { value: "1440" } });
    fireEvent.click(screen.getByTestId("liveness-editor-save"));

    expect(await screen.findByTestId("liveness-editor-error")).toHaveTextContent("Cycle threshold must be a positive whole number.");
    expect(mockCreate).not.toHaveBeenCalled();
  });

  it("AC1: rejects a zero/negative duration client-side without calling the RPC", async () => {
    mockList.mockResolvedValue([]);
    render(<LivenessSection stageSlug="idea" pipelineModeOptions={[]} />);
    await screen.findByText("No liveness overrides configured for this stage.");

    fireEvent.click(screen.getByTestId("liveness-add"));
    fireEvent.change(screen.getByTestId("liveness-editor-expected"), { target: { value: "0" } });
    fireEvent.click(screen.getByTestId("liveness-editor-save"));

    expect(await screen.findByTestId("liveness-editor-error")).toHaveTextContent("Expected duration must be a positive number of minutes.");
    expect(mockCreate).not.toHaveBeenCalled();
  });

  it("AC3: edits a pre-existing per-mode (sdd) override and sends only that kind's fields", async () => {
    const existing = makeDefinition({ id: "sdd-1", stageSlug: "idea", pipelineMode: "sdd", kind: "duration_budget", expectedDurationMs: 45 * 60000, stalenessMarginMs: 5 * 60000 });
    mockList.mockResolvedValue([existing]);
    mockUpdate.mockResolvedValue({ ...existing, expectedDurationMs: 90 * 60000 });

    render(<LivenessSection stageSlug="idea" pipelineModeOptions={[SDD_MODE]} />);
    fireEvent.click(await screen.findByTestId("liveness-row-sdd-edit"));

    // kind/mode lock on edit
    expect(screen.getByTestId("liveness-editor-mode")).toBeDisabled();
    expect(screen.getByTestId("liveness-editor-kind")).toBeDisabled();

    fireEvent.change(screen.getByTestId("liveness-editor-expected"), { target: { value: "90" } });
    fireEvent.click(screen.getByTestId("liveness-editor-save"));

    await waitFor(() =>
      expect(mockUpdate).toHaveBeenCalledWith(
        "sdd-1",
        expect.objectContaining({ expectedDurationMs: 5400000, stalenessMarginMs: 300000, maxNoProgressDurationMs: 0, cycleThreshold: 0, cycleLookbackMs: 0 })
      )
    );
  });

  it("AC1: two-step delete — cancel leaves the row, confirm removes it", async () => {
    const existing = makeDefinition({ id: "d1", stageSlug: "idea", kind: "heartbeat", maxNoProgressDurationMs: 120 * 60000 });
    mockList.mockResolvedValue([existing]);
    mockDelete.mockResolvedValue(true);

    render(<LivenessSection stageSlug="idea" pipelineModeOptions={[]} />);
    expect(await screen.findByTestId("liveness-row-default")).toHaveTextContent("Heartbeat — max no-progress 2h — enabled");
    fireEvent.click(screen.getByTestId("liveness-row-default-delete"));

    fireEvent.click(screen.getByTestId("liveness-row-default-cancel-delete"));
    expect(screen.getByTestId("liveness-row-default")).toBeInTheDocument();
    expect(mockDelete).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId("liveness-row-default-delete"));
    fireEvent.click(screen.getByTestId("liveness-row-default-confirm-delete"));

    await waitFor(() => expect(mockDelete).toHaveBeenCalledWith("d1"));
    await waitFor(() => expect(screen.queryByTestId("liveness-row-default")).not.toBeInTheDocument());
  });

  it("AC1: surfaces a create RPC failure without losing the form", async () => {
    mockList.mockResolvedValue([]);
    mockCreate.mockRejectedValue(new Error("stage not found"));
    render(<LivenessSection stageSlug="idea" pipelineModeOptions={[]} />);
    await screen.findByText("No liveness overrides configured for this stage.");
    fireEvent.click(screen.getByTestId("liveness-add"));
    fireEvent.change(screen.getByTestId("liveness-editor-expected"), { target: { value: "60" } });
    fireEvent.click(screen.getByTestId("liveness-editor-save"));
    expect(await screen.findByTestId("liveness-editor-error")).toHaveTextContent("stage not found");
  });

  it("AC0: surfaces a list-load RPC failure", async () => {
    mockList.mockRejectedValue(new Error("boom"));
    render(<LivenessSection stageSlug="idea" pipelineModeOptions={[]} />);
    expect(await screen.findByTestId("liveness-section-error")).toHaveTextContent("boom");
  });

  it("AC1: creates a new cycle_frequency override with threshold/lookback fields", async () => {
    mockList.mockResolvedValue([]);
    const created = makeDefinition({ id: "new-2", stageSlug: "review", kind: "cycle_frequency", cycleThreshold: 3, cycleLookbackMs: 24 * 3600000 });
    mockCreate.mockResolvedValue(created);

    render(<LivenessSection stageSlug="review" pipelineModeOptions={[]} />);
    await screen.findByText("No liveness overrides configured for this stage.");

    fireEvent.click(screen.getByTestId("liveness-add"));
    fireEvent.change(screen.getByTestId("liveness-editor-kind"), { target: { value: "cycle_frequency" } });
    fireEvent.change(screen.getByTestId("liveness-editor-threshold"), { target: { value: "3" } });
    fireEvent.change(screen.getByTestId("liveness-editor-lookback"), { target: { value: "1440" } });
    fireEvent.click(screen.getByTestId("liveness-editor-save"));

    await waitFor(() =>
      expect(mockCreate).toHaveBeenCalledWith(
        expect.objectContaining({ stageSlug: "review", kind: "cycle_frequency", cycleThreshold: 3, cycleLookbackMs: 86400000 })
      )
    );
    expect(await screen.findByTestId("liveness-row-default")).toHaveTextContent("3 cycles / 24h lookback");
  });

  it("AC1: cancels the new-row editor without saving", async () => {
    mockList.mockResolvedValue([]);
    render(<LivenessSection stageSlug="idea" pipelineModeOptions={[]} />);
    await screen.findByText("No liveness overrides configured for this stage.");

    fireEvent.click(screen.getByTestId("liveness-add"));
    expect(screen.getByTestId("liveness-row-editor")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("liveness-editor-cancel"));

    expect(screen.queryByTestId("liveness-row-editor")).not.toBeInTheDocument();
    expect(mockCreate).not.toHaveBeenCalled();
  });

  it("AC0: with no base row and no built-in default, states that no liveness check runs", async () => {
    mockList.mockResolvedValue([]);
    render(<LivenessSection stageSlug="queued" pipelineModeOptions={[]} />);
    await screen.findByText("No liveness overrides configured for this stage.");
    expect(screen.getByTestId("liveness-fallback-note")).toHaveTextContent(
      "No stage-wide default configured — modes without an override have no timeout (no liveness check runs)."
    );
  });
});
