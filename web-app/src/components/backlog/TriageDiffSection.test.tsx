/**
 * Tests for TriageDiffSection's per-question answer form (Epic 1.1, Story
 * 1.1.1, Task 1.1.1h) — inline "Answer ▸" toggle + textarea per rendered
 * clarifying question, composed into a "Q:.../A:..." feedback string on
 * submit via onAnswerQuestion.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TriageDiffSection } from "./TriageDiffSection";
import type { AcCriterion, TriageSuggestion } from "@/lib/hooks/useBacklogService";

const CURRENT_CRITERIA: AcCriterion[] = [];

const QUESTION_SUGGESTION: TriageSuggestion = {
  text: "Should retries be per-workflow or global?",
  rationale: "question",
};

function renderWithQuestion(
  onAnswerQuestion?: (feedback: string) => Promise<void>,
  suggestions: TriageSuggestion[] = [QUESTION_SUGGESTION]
) {
  return render(
    <TriageDiffSection
      currentCriteria={CURRENT_CRITERIA}
      suggestedSuggestions={suggestions}
      onAnswerQuestion={onAnswerQuestion}
    />
  );
}

describe("TriageDiffSection_question_answer_toggle", () => {
  it("opens the answer form with aria-expanded=true and closes it again", () => {
    renderWithQuestion(jest.fn());

    const toggle = screen.getByTestId("triage-question-answer-toggle-0");
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByTestId("triage-question-answer-input-0")).not.toBeInTheDocument();

    fireEvent.click(toggle);

    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByTestId("triage-question-answer-input-0")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("triage-question-answer-cancel-0"));

    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByTestId("triage-question-answer-input-0")).not.toBeInTheDocument();
  });

  it("renders nothing extra when onAnswerQuestion is absent (read-only historical mode)", () => {
    renderWithQuestion(undefined);

    expect(screen.getByText(QUESTION_SUGGESTION.text)).toBeInTheDocument();
    expect(screen.queryByTestId("triage-question-answer-toggle-0")).not.toBeInTheDocument();
  });
});

describe("TriageDiffSection_question_answer_submit", () => {
  it("disables Submit until the answer textarea is non-empty", () => {
    renderWithQuestion(jest.fn());
    fireEvent.click(screen.getByTestId("triage-question-answer-toggle-0"));

    const submit = screen.getByTestId("triage-question-answer-submit-0");
    expect(submit).toBeDisabled();

    fireEvent.change(screen.getByTestId("triage-question-answer-input-0"), {
      target: { value: "Per-workflow, default to global" },
    });

    expect(submit).not.toBeDisabled();
  });

  it("calls onAnswerQuestion with the exact composed Q:/A: string", async () => {
    const onAnswerQuestion = jest.fn().mockResolvedValue(undefined);
    renderWithQuestion(onAnswerQuestion);

    fireEvent.click(screen.getByTestId("triage-question-answer-toggle-0"));
    fireEvent.change(screen.getByTestId("triage-question-answer-input-0"), {
      target: { value: "Per-workflow, default to global" },
    });
    fireEvent.click(screen.getByTestId("triage-question-answer-submit-0"));

    await waitFor(() => {
      expect(onAnswerQuestion).toHaveBeenCalledWith(
        "Q: Should retries be per-workflow or global?\nA: Per-workflow, default to global"
      );
    });
  });

  it("renders '✓ Answered' after a successful submit", async () => {
    const onAnswerQuestion = jest.fn().mockResolvedValue(undefined);
    renderWithQuestion(onAnswerQuestion);

    fireEvent.click(screen.getByTestId("triage-question-answer-toggle-0"));
    fireEvent.change(screen.getByTestId("triage-question-answer-input-0"), {
      target: { value: "Per-workflow, default to global" },
    });
    fireEvent.click(screen.getByTestId("triage-question-answer-submit-0"));

    await waitFor(() => {
      expect(screen.getByText(/✓ Answered: Per-workflow, default to global/)).toBeInTheDocument();
    });
    expect(screen.queryByTestId("triage-question-answer-toggle-0")).not.toBeInTheDocument();
  });

  it("surfaces an inline error and keeps the answer when onAnswerQuestion rejects", async () => {
    const onAnswerQuestion = jest.fn().mockRejectedValue(new Error("Retriage already in flight"));
    renderWithQuestion(onAnswerQuestion);

    fireEvent.click(screen.getByTestId("triage-question-answer-toggle-0"));
    fireEvent.change(screen.getByTestId("triage-question-answer-input-0"), {
      target: { value: "Per-workflow, default to global" },
    });
    fireEvent.click(screen.getByTestId("triage-question-answer-submit-0"));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("Retriage already in flight");
    });
    expect(screen.getByRole("alert")).toHaveTextContent("Failed to submit answer");
    // The answer was not silently dropped — the form (and its typed draft) is still there.
    expect(screen.getByTestId("triage-question-answer-input-0")).toHaveValue(
      "Per-workflow, default to global"
    );
  });

  it("retries the submit with the same draft when Retry is clicked after a failure", async () => {
    const onAnswerQuestion = jest
      .fn()
      .mockRejectedValueOnce(new Error("Retriage already in flight"))
      .mockResolvedValueOnce(undefined);
    renderWithQuestion(onAnswerQuestion);

    fireEvent.click(screen.getByTestId("triage-question-answer-toggle-0"));
    fireEvent.change(screen.getByTestId("triage-question-answer-input-0"), {
      target: { value: "Per-workflow, default to global" },
    });
    fireEvent.click(screen.getByTestId("triage-question-answer-submit-0"));

    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Retry submitting answer" }));

    await waitFor(() => {
      expect(onAnswerQuestion).toHaveBeenCalledTimes(2);
      expect(screen.getByText(/✓ Answered: Per-workflow, default to global/)).toBeInTheDocument();
    });
  });

  it("dismisses the form (discarding the draft) when the error's dismiss button is clicked", async () => {
    const onAnswerQuestion = jest.fn().mockRejectedValue(new Error("Retriage already in flight"));
    renderWithQuestion(onAnswerQuestion);

    const toggle = screen.getByTestId("triage-question-answer-toggle-0");
    fireEvent.click(toggle);
    fireEvent.change(screen.getByTestId("triage-question-answer-input-0"), {
      target: { value: "Per-workflow, default to global" },
    });
    fireEvent.click(screen.getByTestId("triage-question-answer-submit-0"));

    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Dismiss error" }));

    expect(screen.queryByTestId("triage-question-answer-input-0")).not.toBeInTheDocument();
    expect(toggle).toHaveAttribute("aria-expanded", "false");
  });

  it("returns focus to the toggle button after Cancel", () => {
    renderWithQuestion(jest.fn());
    const toggle = screen.getByTestId("triage-question-answer-toggle-0");

    fireEvent.click(toggle);
    fireEvent.click(screen.getByTestId("triage-question-answer-cancel-0"));

    expect(toggle).toHaveFocus();
  });

  it("cancels the open form on Escape the same way Cancel does", () => {
    renderWithQuestion(jest.fn());
    fireEvent.click(screen.getByTestId("triage-question-answer-toggle-0"));

    fireEvent.keyDown(screen.getByTestId("triage-question-answer-input-0"), { key: "Escape" });

    expect(screen.queryByTestId("triage-question-answer-input-0")).not.toBeInTheDocument();
  });
});
