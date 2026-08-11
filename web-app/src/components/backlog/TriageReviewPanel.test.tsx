/**
 * Tests for TriageReviewPanel component (T-12, cases 6–11).
 *
 * mapBacklogItem's triageStatus derivation logic is tested directly against
 * the real function in useBacklogService.test.ts — it is not re-tested here.
 */

import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { TriageReviewPanel } from "./TriageReviewPanel";
import type { BacklogItem, TriageResult, AcCriterion } from "@/lib/hooks/useBacklogService";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const TRIAGE_RESULT_WITH_SUGGESTIONS: TriageResult = {
  summary: "Item looks implementable. Suggested AC below.",
  suggestions: [
    { text: "User sees confirmation on submit", rationale: "UX clarity" },
    { text: "Errors are displayed inline", rationale: "accessibility" },
  ],
  clarifyingQuestions: [],
};

const TRIAGE_RESULT_NO_SUGGESTIONS: TriageResult = {
  summary: "Item is well-defined. No AC changes needed.",
  suggestions: [],
  clarifyingQuestions: [],
};

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-001",
    title: "Test item",
    status: "idea",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    statusEvents: [],
    progressNotes: [],
    totalEstimatedCostUsd: 0,
    triageStatus: "completed",
    triageResult: TRIAGE_RESULT_WITH_SUGGESTIONS,
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Local storage helpers
// ---------------------------------------------------------------------------

beforeEach(() => {
  localStorage.clear();
});

// ---------------------------------------------------------------------------
// Test 6: TriageReviewPanel_renders_diff_when_suggestions_present
// ---------------------------------------------------------------------------

describe("TriageReviewPanel_renders_diff_when_suggestions_present", () => {
  it("renders the triage review panel and summary when suggestions are present", () => {
    render(
      <TriageReviewPanel
        item={makeItem()}
        triageResult={TRIAGE_RESULT_WITH_SUGGESTIONS}
        onApply={jest.fn()}
        onSkip={jest.fn()}
      />
    );

    expect(screen.getByTestId("triage-review-panel")).toBeInTheDocument();
    expect(screen.getByText(TRIAGE_RESULT_WITH_SUGGESTIONS.summary)).toBeInTheDocument();
    expect(screen.getByTestId("triage-apply-button")).toBeInTheDocument();
  });

  it("renders suggested AC criteria text in the diff section", () => {
    render(
      <TriageReviewPanel
        item={makeItem()}
        triageResult={TRIAGE_RESULT_WITH_SUGGESTIONS}
        onApply={jest.fn()}
        onSkip={jest.fn()}
      />
    );

    expect(screen.getByText("User sees confirmation on submit")).toBeInTheDocument();
    expect(screen.getByText("Errors are displayed inline")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test 7: TriageReviewPanel_renders_summary_only_when_no_suggestions
// ---------------------------------------------------------------------------

describe("TriageReviewPanel_renders_summary_only_when_no_suggestions", () => {
  it("shows 'Mark ready' button and no-suggestions text when suggestions list is empty", () => {
    render(
      <TriageReviewPanel
        item={makeItem({ triageResult: TRIAGE_RESULT_NO_SUGGESTIONS })}
        triageResult={TRIAGE_RESULT_NO_SUGGESTIONS}
        onApply={jest.fn()}
        onSkip={jest.fn()}
      />
    );

    expect(screen.getByTestId("triage-mark-ready-button")).toBeInTheDocument();
    expect(screen.getByText(/No AC changes suggested/i)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test 8: TriageReviewPanel_does_not_render_when_dismissed_in_localStorage
// ---------------------------------------------------------------------------

describe("TriageReviewPanel_does_not_render_when_dismissed_in_localStorage", () => {
  it("returns null when the dismissal key for this item is set in localStorage", () => {
    localStorage.setItem("triage-panel-dismissed-item-001", "1");

    const { queryByTestId } = render(
      <TriageReviewPanel
        item={makeItem()}
        triageResult={TRIAGE_RESULT_WITH_SUGGESTIONS}
        onApply={jest.fn()}
        onSkip={jest.fn()}
      />
    );

    expect(queryByTestId("triage-review-panel")).not.toBeInTheDocument();
  });

  it("renders normally when no dismissal key exists in localStorage", () => {
    const { queryByTestId } = render(
      <TriageReviewPanel
        item={makeItem()}
        triageResult={TRIAGE_RESULT_WITH_SUGGESTIONS}
        onApply={jest.fn()}
        onSkip={jest.fn()}
      />
    );

    expect(queryByTestId("triage-review-panel")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test 9: TriageReviewPanel_apply_calls_onApply_with_cached_criteria
// ---------------------------------------------------------------------------

describe("TriageReviewPanel_apply_calls_updateBacklogItem_then_transitionStatus", () => {
  it("calls onApply when the apply button is clicked", async () => {
    const onApply = jest.fn().mockResolvedValue(undefined);
    const item = makeItem({
      acCriteria: [{ index: 0, text: "Existing AC", status: "pending" }],
    });

    render(
      <TriageReviewPanel
        item={item}
        triageResult={TRIAGE_RESULT_WITH_SUGGESTIONS}
        onApply={onApply}
        onSkip={jest.fn()}
      />
    );

    fireEvent.click(screen.getByTestId("triage-apply-button"));

    await waitFor(() => {
      expect(onApply).toHaveBeenCalledTimes(1);
    });

    // The pre-apply criteria (item.acCriteria snapshot) are passed to onApply
    expect(onApply).toHaveBeenCalledWith(
      expect.arrayContaining([
        expect.objectContaining({ text: "Existing AC" }),
      ])
    );
  });
});

// ---------------------------------------------------------------------------
// Test 10: TriageReviewPanel_shows_error_banner_on_apply_failure
// ---------------------------------------------------------------------------

describe("TriageReviewPanel_shows_error_banner_on_apply_failure", () => {
  it("shows the error banner when onApply rejects", async () => {
    const onApply = jest.fn().mockRejectedValue(new Error("Network error"));

    render(
      <TriageReviewPanel
        item={makeItem()}
        triageResult={TRIAGE_RESULT_WITH_SUGGESTIONS}
        onApply={onApply}
        onSkip={jest.fn()}
      />
    );

    await act(async () => {
      fireEvent.click(screen.getByTestId("triage-apply-button"));
    });

    await waitFor(() => {
      expect(screen.getByTestId("triage-error-banner")).toBeInTheDocument();
    });

    expect(screen.getByText(/Network error/i)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test 11: TriageReviewPanel_shows_undo_toast_on_apply_success
// ---------------------------------------------------------------------------

describe("TriageReviewPanel_shows_undo_toast_on_apply_success", () => {
  it("shows the undo toast after successful apply", async () => {
    const onApply = jest.fn().mockResolvedValue(undefined);

    render(
      <TriageReviewPanel
        item={makeItem()}
        triageResult={TRIAGE_RESULT_WITH_SUGGESTIONS}
        onApply={onApply}
        onSkip={jest.fn()}
      />
    );

    await act(async () => {
      fireEvent.click(screen.getByTestId("triage-apply-button"));
    });

    await waitFor(() => {
      expect(screen.getByTestId("triage-undo-toast")).toBeInTheDocument();
    });

    expect(screen.getByTestId("triage-undo-button")).toBeInTheDocument();
  });

  it("calls onUndoApply with pre-apply criteria when Undo is clicked", async () => {
    const onApply = jest.fn().mockResolvedValue(undefined);
    const onUndoApply = jest.fn().mockResolvedValue(undefined);
    const item = makeItem({
      acCriteria: [{ index: 0, text: "Original AC", status: "pending" }],
    });

    render(
      <TriageReviewPanel
        item={item}
        triageResult={TRIAGE_RESULT_WITH_SUGGESTIONS}
        onApply={onApply}
        onUndoApply={onUndoApply}
        onSkip={jest.fn()}
      />
    );

    await act(async () => {
      fireEvent.click(screen.getByTestId("triage-apply-button"));
    });

    await waitFor(() => {
      expect(screen.getByTestId("triage-undo-button")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByTestId("triage-undo-button"));

    await waitFor(() => {
      expect(onUndoApply).toHaveBeenCalledTimes(1);
    });

    expect(onUndoApply).toHaveBeenCalledWith(
      expect.arrayContaining([
        expect.objectContaining({ text: "Original AC" }),
      ])
    );
  });
});

// ---------------------------------------------------------------------------
// TriageReviewPanel_refine_with_feedback
// ---------------------------------------------------------------------------

describe("TriageReviewPanel_refine_with_feedback", () => {
  it("does not show the refine toggle when onRefine is not provided", () => {
    render(
      <TriageReviewPanel
        item={makeItem()}
        triageResult={TRIAGE_RESULT_WITH_SUGGESTIONS}
        onApply={jest.fn()}
        onSkip={jest.fn()}
      />
    );

    expect(screen.queryByTestId("triage-refine-toggle-button")).not.toBeInTheDocument();
  });

  it("submits typed feedback via onRefine when Refine triage is clicked", async () => {
    const onRefine = jest.fn().mockResolvedValue(undefined);

    render(
      <TriageReviewPanel
        item={makeItem()}
        triageResult={TRIAGE_RESULT_WITH_SUGGESTIONS}
        onApply={jest.fn()}
        onSkip={jest.fn()}
        onRefine={onRefine}
      />
    );

    fireEvent.click(screen.getByTestId("triage-refine-toggle-button"));
    fireEvent.change(screen.getByTestId("triage-refine-textarea"), {
      target: { value: "Missed the mobile case entirely." },
    });

    await act(async () => {
      fireEvent.click(screen.getByTestId("triage-refine-submit-button"));
    });

    await waitFor(() => {
      expect(onRefine).toHaveBeenCalledWith("Missed the mobile case entirely.");
    });
  });

  it("disables the refine submit button until feedback is typed", () => {
    render(
      <TriageReviewPanel
        item={makeItem()}
        triageResult={TRIAGE_RESULT_WITH_SUGGESTIONS}
        onApply={jest.fn()}
        onSkip={jest.fn()}
        onRefine={jest.fn()}
      />
    );

    fireEvent.click(screen.getByTestId("triage-refine-toggle-button"));
    expect(screen.getByTestId("triage-refine-submit-button")).toBeDisabled();
  });

  it("shows an iteration badge when triageResult.iteration is greater than 1", () => {
    render(
      <TriageReviewPanel
        item={makeItem()}
        triageResult={{ ...TRIAGE_RESULT_WITH_SUGGESTIONS, iteration: 2 }}
        onApply={jest.fn()}
        onSkip={jest.fn()}
      />
    );

    expect(screen.getByText(/Iteration 2/)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Story 4.1.2: readOnly mode (Structured Diagnostic for Headless Diagnostic
// Sessions)
// ---------------------------------------------------------------------------

describe("TriageReviewPanel_should_HideApplySkipRefineButtonsAndShowSummarySuggestionsTasks_When_ReadOnlyIsTrue", () => {
  it("omits Apply/Skip/Refine/dismiss buttons but keeps summary, suggestions, and task list", () => {
    // readOnly props have no write-mode callbacks at all (discriminated
    // union) — nothing to fabricate noop stand-ins for.
    render(
      <TriageReviewPanel
        item={makeItem()}
        triageResult={{
          ...TRIAGE_RESULT_WITH_SUGGESTIONS,
          tasks: [{ text: "Reworded AC #2", estimate: "", category: "" }],
        }}
        readOnly
      />
    );

    // Panel and its informational content are present.
    expect(screen.getByTestId("triage-review-panel")).toBeInTheDocument();
    expect(screen.getByText(TRIAGE_RESULT_WITH_SUGGESTIONS.summary)).toBeInTheDocument();
    expect(screen.getByText("User sees confirmation on submit")).toBeInTheDocument();
    expect(screen.getByTestId("triage-task-list")).toBeInTheDocument();
    expect(screen.getByText("Reworded AC #2")).toBeInTheDocument();

    // Action buttons absent from the DOM (not merely disabled).
    expect(screen.queryByTestId("triage-apply-button")).not.toBeInTheDocument();
    expect(screen.queryByTestId("triage-mark-ready-button")).not.toBeInTheDocument();
    expect(screen.queryByTestId("triage-skip-button")).not.toBeInTheDocument();
    expect(screen.queryByTestId("triage-refine-toggle-button")).not.toBeInTheDocument();
    expect(screen.queryByTestId("triage-dismiss-button")).not.toBeInTheDocument();
  });

  it("ignores a pre-existing localStorage dismissal for the same item.id — a historical record is never dismissible", () => {
    // Simulates the user having dismissed the live interactive triage panel
    // for this item — the readOnly historical-record render must not
    // inherit that dismissed state (they share the same localStorage key).
    localStorage.setItem("triage-panel-dismissed-item-001", "1");

    render(
      <TriageReviewPanel
        item={makeItem()}
        triageResult={TRIAGE_RESULT_WITH_SUGGESTIONS}
        readOnly
      />
    );

    expect(screen.getByTestId("triage-review-panel")).toBeInTheDocument();
  });
});
