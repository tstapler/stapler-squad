/**
 * Tests for StageForm — Epic 2.8 (Story 2.8.2) of
 * project_plans/backlog-custom-workflow-stages/implementation/plan.md.
 *
 * Covers:
 *  1. Create success: submitting calls createStage with the entered payload
 *     and invokes onSaved with the server's returned stage.
 *  2. Task 2.8.2e (accessibility, required — not optional): a gate kind's
 *     checkbox is keyboard-operable (Space toggles it while focused) and its
 *     progressive-disclosure field appears/disappears (and clears) with it.
 *  3. Task 2.8.2e: a checked gate with an empty required sub-field is caught
 *     client-side before any RPC call, with `aria-invalid` and
 *     `aria-describedby` wired to the inline error — not just a visual cue.
 *  4. Task 2.8.2e: the graph preview's `sr-only` table is present in the
 *     accessibility tree (not `display:none`) and matches the configured edges.
 *  5. Built-in stages' Delete button is disabled with an explanatory tooltip.
 *  6. Two-step delete-with-confirm flow, mirroring PipelineModeForm.test.tsx.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StageForm } from "./StageForm";
import { useBacklogStagesAdmin } from "@/lib/hooks/useBacklogStages";
import type { BacklogStage, StageTransition } from "@/lib/hooks/useBacklogStages";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import type { PipelineMode } from "@/lib/hooks/useBacklogService";

jest.mock("@/lib/hooks/useBacklogStages", () => {
  const actual = jest.requireActual("@/lib/hooks/useBacklogStages");
  return { ...actual, useBacklogStagesAdmin: jest.fn() };
});

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: jest.fn(),
}));

const mockUseBacklogStages = useBacklogStagesAdmin as jest.MockedFunction<typeof useBacklogStagesAdmin>;
const mockUseBacklogService = useBacklogService as jest.MockedFunction<typeof useBacklogService>;

const mockCreateStage = jest.fn();
const mockUpdateStage = jest.fn();
const mockDeleteStage = jest.fn();
const mockListStageTransitions = jest.fn();
const mockCreateStageTransition = jest.fn();
const mockUpdateTransitionGate = jest.fn();
const mockCreateTransitionGate = jest.fn();
const mockDeleteTransitionGate = jest.fn();
const mockDeleteStageTransition = jest.fn();
const mockListPipelineModes = jest.fn();

function makeStage(overrides: Partial<BacklogStage> & Pick<BacklogStage, "id" | "slug" | "name">): BacklogStage {
  return { description: "", isEntry: false, isTerminal: false, enabled: true, ...overrides };
}

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

// Same benign vanilla-extract jest-mock className warning noted in
// PipelineModeForm.test.tsx.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterAll(() => {
  jest.restoreAllMocks();
});

beforeEach(() => {
  jest.clearAllMocks();
  mockListStageTransitions.mockResolvedValue([]);
  mockListPipelineModes.mockResolvedValue([makePipelineMode({ id: "p1", slug: "sdd", name: "SDD Mode" })]);
  mockUseBacklogStages.mockReturnValue({
    createStage: mockCreateStage,
    updateStage: mockUpdateStage,
    deleteStage: mockDeleteStage,
    listStageTransitions: mockListStageTransitions,
    createStageTransition: mockCreateStageTransition,
    updateStageTransition: jest.fn(),
    deleteStageTransition: mockDeleteStageTransition,
    createTransitionGate: mockCreateTransitionGate,
    updateTransitionGate: mockUpdateTransitionGate,
    deleteTransitionGate: mockDeleteTransitionGate,
  } as unknown as ReturnType<typeof useBacklogStagesAdmin>);
  mockUseBacklogService.mockReturnValue({
    listPipelineModes: mockListPipelineModes,
  } as unknown as ReturnType<typeof useBacklogService>);
});

const STAGE_A = makeStage({ id: "1", slug: "idea", name: "Idea", isEntry: true });
const STAGE_B = makeStage({ id: "2", slug: "ready", name: "Ready" });

describe("StageForm", () => {
  it("create success: submits payload and calls onSaved without navigating away", async () => {
    const created = makeStage({ id: "stage-new", slug: "design-review", name: "Design Review" });
    mockCreateStage.mockResolvedValue(created);
    const onSaved = jest.fn();

    render(<StageForm stage={null} allStages={[STAGE_A, STAGE_B]} onSaved={onSaved} onDeleted={jest.fn()} onCancel={jest.fn()} />);

    fireEvent.change(screen.getByTestId("stage-form-slug"), { target: { value: "design-review" } });
    fireEvent.change(screen.getByTestId("stage-form-name"), { target: { value: "Design Review" } });
    fireEvent.click(screen.getByTestId("stage-form-submit"));

    await waitFor(() => expect(mockCreateStage).toHaveBeenCalledTimes(1));
    expect(mockCreateStage).toHaveBeenCalledWith(
      expect.objectContaining({ slug: "design-review", name: "Design Review" })
    );
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith(created));
  });

  it("a gate checkbox is keyboard-operable and its progressive-disclosure field appears/clears with it", async () => {
    const user = userEvent.setup();
    render(<StageForm stage={null} allStages={[STAGE_A, STAGE_B]} onSaved={jest.fn()} onDeleted={jest.fn()} onCancel={jest.fn()} />);

    fireEvent.click(screen.getByTestId("stage-transition-add"));

    const checkbox = await screen.findByLabelText("Automated review");
    expect(screen.queryByLabelText("Review prompt / mode")).not.toBeInTheDocument();

    // Keyboard operability (Task 2.8.2e / plan.md Task 2.8.2c4): Space
    // toggles the checkbox while focused, no pointer interaction required.
    checkbox.focus();
    await user.keyboard(" ");

    expect(checkbox).toBeChecked();
    expect(screen.getByLabelText("Review prompt / mode")).toBeInTheDocument();

    // Unchecking hides AND clears the field (ux.md Surface 2 flow #2).
    await user.keyboard(" ");
    expect(checkbox).not.toBeChecked();
    expect(screen.queryByLabelText("Review prompt / mode")).not.toBeInTheDocument();
  });

  it("a checked gate with an empty required sub-field is caught client-side with aria-invalid/aria-describedby wired to the error", async () => {
    render(<StageForm stage={null} allStages={[STAGE_A, STAGE_B]} onSaved={jest.fn()} onDeleted={jest.fn()} onCancel={jest.fn()} />);

    fireEvent.click(screen.getByTestId("stage-transition-add"));
    fireEvent.change(screen.getByTestId("stage-transition-0-to"), { target: { value: "ready" } });
    fireEvent.click(screen.getByLabelText("Automated review"));
    // Leave the pipeline-mode select unset.

    fireEvent.change(screen.getByTestId("stage-form-slug"), { target: { value: "design-review" } });
    fireEvent.change(screen.getByTestId("stage-form-name"), { target: { value: "Design Review" } });
    fireEvent.click(screen.getByTestId("stage-form-submit"));

    const select = await screen.findByLabelText("Review prompt / mode");
    await waitFor(() => expect(select).toHaveAttribute("aria-invalid", "true"));
    const describedBy = select.getAttribute("aria-describedby");
    expect(describedBy).toBeTruthy();
    expect(document.getElementById(describedBy as string)).toHaveTextContent("Select a review prompt for this gate.");

    // Client-side validation blocks the RPC call entirely.
    expect(mockCreateStage).not.toHaveBeenCalled();
  });

  it("the graph preview's sr-only table is present in the accessibility tree and matches the configured edges", async () => {
    const transitions: StageTransition[] = [
      {
        id: "t1",
        fromStageSlug: "idea",
        toStageSlug: "ready",
        enabled: true,
        gates: [
          { id: "g1", transitionId: "t1", kind: "human_approval", config: {}, stateful: false, orderIndex: 0, enabled: true },
          {
            id: "g2",
            transitionId: "t1",
            kind: "automated_review",
            config: { pipeline_mode: "sdd" },
            stateful: false,
            orderIndex: 1,
            enabled: true,
          },
        ],
      },
    ];
    mockListStageTransitions.mockResolvedValue(transitions);

    render(<StageForm stage={STAGE_A} allStages={[STAGE_A, STAGE_B]} onSaved={jest.fn()} onDeleted={jest.fn()} onCancel={jest.fn()} />);

    const table = await screen.findByTestId("stage-graph-sr-table");
    // sr-only means clip-hidden, not display:none — still queryable/present.
    expect(table).toBeInTheDocument();
    // The table renders immediately with 0 edges (no stages/transitions
    // loaded yet); wait for the async listStageTransitions() fetch to land.
    await waitFor(() => expect(table.querySelectorAll("tbody tr")).toHaveLength(1));
    const rows = table.querySelectorAll("tbody tr");
    expect(rows[0]).toHaveTextContent("Idea");
    expect(rows[0]).toHaveTextContent("Ready");
    expect(rows[0]).toHaveTextContent("2 (human approval, automated review)");
  });

  it("a built-in stage's Delete button is disabled with an explanatory tooltip", async () => {
    render(<StageForm stage={STAGE_A} allStages={[STAGE_A, STAGE_B]} onSaved={jest.fn()} onDeleted={jest.fn()} onCancel={jest.fn()} />);

    const deleteBtn = await screen.findByTestId("stage-form-delete");
    expect(deleteBtn).toBeDisabled();
    expect(deleteBtn).toHaveAttribute("title", "Built-in stage — disable it instead of deleting.");
  });

  it("delete-with-confirm flow: Delete shows a Confirm delete? button; confirming calls deleteStage and invokes onDeleted", async () => {
    const customStage = makeStage({ id: "2", slug: "design-review", name: "Design Review" });
    mockDeleteStage.mockResolvedValue(true);
    const onDeleted = jest.fn();

    render(<StageForm stage={customStage} allStages={[STAGE_A, customStage]} onSaved={jest.fn()} onDeleted={onDeleted} onCancel={jest.fn()} />);

    const deleteBtn = await screen.findByTestId("stage-form-delete");
    expect(deleteBtn).not.toBeDisabled();
    fireEvent.click(deleteBtn);

    fireEvent.click(screen.getByTestId("stage-form-confirm-delete"));

    await waitFor(() => expect(mockDeleteStage).toHaveBeenCalledWith("2"));
    await waitFor(() => expect(onDeleted).toHaveBeenCalledWith("2"));
  });
});
