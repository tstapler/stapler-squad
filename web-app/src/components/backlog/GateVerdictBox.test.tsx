/**
 * Tests for GateVerdictBox component.
 *
 * Covers:
 *  1. PASS verdict renders "PASSED" label and primary "Approve" button
 *  2. PARTIAL verdict renders "PARTIAL" label, "Reopen for Revision" primary button, no "Approve" button
 *  3. FAIL verdict renders "FAILED" label, "Reopen for Revision" primary button
 *  4. PENDING verdict renders "PENDING" label, disabled Approve and Reopen buttons with aria-disabled
 *  5. Ctrl+Enter on PASS verdict calls onApprove
 *  6. Ctrl+Enter on PARTIAL verdict calls onReopen (NOT onApprove)
 *  7. Override form toggle shows/hides the textarea on click
 *  8. Override submit button is disabled when reason.length < 5
 *  9. Override submit button is enabled when reason.length >= 5
 *  10. Submitting override calls onOverride with the reason
 *  11. Skip gate: clicking "Skip gate..." link shows the inline confirmation
 *  12. Skip gate: clicking Cancel in confirmation hides it
 *  13. Skip gate: clicking "Confirm — Skip Gate" calls onSkipGate
 *  14. Skip gate: Escape key in confirmation hides it
 *  15. Skip gate: focus trap — Tab cycles between Cancel and Confirm only
 *  16. Criteria list renders when verdict is PARTIAL with criteria
 *  17. UNVERIFIABLE verdict + onReReview provided renders "Re-run Gate" and clicking it calls onReReview
 *  18. UNVERIFIABLE verdict + onReReview omitted hides "Re-run Gate"
 *  19. UNVERIFIABLE verdict renders "Reopen for Revision" and the Override section
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { GateVerdictBox, type GateVerdictBoxWriteProps } from "./GateVerdictBox";

// ---------------------------------------------------------------------------
// Default props factory
// ---------------------------------------------------------------------------

function makeProps(overrides: Partial<GateVerdictBoxWriteProps> = {}): GateVerdictBoxWriteProps {
  return {
    verdict: "PASS" as const,
    summary: "All checks passed.",
    onApprove: jest.fn().mockResolvedValue(undefined),
    onReopen: jest.fn().mockResolvedValue(undefined),
    onOverride: jest.fn().mockResolvedValue(undefined),
    onSkipGate: jest.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Test: 1 — PASS verdict
// ---------------------------------------------------------------------------

describe("GateVerdictBox — PASS verdict", () => {
  it("renders PASSED label and primary Approve button", () => {
    render(<GateVerdictBox {...makeProps({ verdict: "PASS" })} />);

    expect(screen.getByText("PASSED")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Approve/i })).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test: 2 — PARTIAL verdict
// ---------------------------------------------------------------------------

describe("GateVerdictBox — PARTIAL verdict", () => {
  it("renders PARTIAL label and Reopen button but no Approve button", () => {
    render(<GateVerdictBox {...makeProps({ verdict: "PARTIAL" })} />);

    expect(screen.getByText("PARTIAL")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Reopen for Revision/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Approve — Mark Done/i })).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test: 3 — FAIL verdict
// ---------------------------------------------------------------------------

describe("GateVerdictBox — FAIL verdict", () => {
  it("renders FAILED label and Reopen for Revision button", () => {
    render(<GateVerdictBox {...makeProps({ verdict: "FAIL" })} />);

    expect(screen.getByText("FAILED")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Reopen for Revision/i })).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test: 4 — PENDING verdict
// ---------------------------------------------------------------------------

describe("GateVerdictBox — PENDING verdict", () => {
  it("renders PENDING label with disabled Approve button; Reopen stays enabled", () => {
    render(<GateVerdictBox {...makeProps({ verdict: "PENDING" })} />);

    expect(screen.getByText("PENDING")).toBeInTheDocument();

    const approveBtn = screen.getByRole("button", { name: /Approve — Mark Done/i });
    expect(approveBtn).toBeDisabled();
    expect(approveBtn).toHaveAttribute("aria-disabled", "true");

    // Reopen is intentionally enabled during PENDING (fix(backlog): enable Reopen in pending state)
    const reopenBtn = screen.getByRole("button", { name: /Reopen for Revision/i });
    expect(reopenBtn).not.toBeDisabled();
  });
});

// ---------------------------------------------------------------------------
// Test: 5 — Ctrl+Enter on PASS calls onApprove
// ---------------------------------------------------------------------------

describe("GateVerdictBox — keyboard shortcut", () => {
  it("Ctrl+Enter on PASS verdict calls onApprove", async () => {
    const onApprove = jest.fn().mockResolvedValue(undefined);
    const onReopen = jest.fn().mockResolvedValue(undefined);
    render(
      <GateVerdictBox
        {...makeProps({ verdict: "PASS", onApprove, onReopen })}
      />
    );

    const section = screen.getByRole("status", { name: "Gate verdict" });
    fireEvent.keyDown(section, { key: "Enter", ctrlKey: true });

    await waitFor(() => expect(onApprove).toHaveBeenCalledTimes(1));
    expect(onReopen).not.toHaveBeenCalled();
  });

  // ---------------------------------------------------------------------------
  // Test: 6 — Ctrl+Enter on PARTIAL calls onReopen NOT onApprove
  // ---------------------------------------------------------------------------

  it("Ctrl+Enter on PARTIAL verdict opens reopen form, not onApprove", async () => {
    const onApprove = jest.fn().mockResolvedValue(undefined);
    const onReopen = jest.fn().mockResolvedValue(undefined);
    render(
      <GateVerdictBox
        {...makeProps({ verdict: "PARTIAL", onApprove, onReopen })}
      />
    );

    const section = screen.getByRole("status", { name: "Gate verdict" });
    fireEvent.keyDown(section, { key: "Enter", ctrlKey: true });

    await waitFor(() =>
      expect(screen.getByRole("form", { name: /Reopen for revision/i })).toBeInTheDocument()
    );
    expect(onApprove).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Test: 7 — Override form toggle shows/hides textarea
// ---------------------------------------------------------------------------

describe("GateVerdictBox — override form", () => {
  it("toggle shows and hides the override textarea", async () => {
    render(<GateVerdictBox {...makeProps({ verdict: "PARTIAL" })} />);

    // textarea not visible initially
    expect(screen.queryByLabelText(/Reason for override/i)).not.toBeInTheDocument();

    // click toggle to open
    const toggle = screen.getByRole("button", { name: /Override: Mark done anyway/i });
    fireEvent.click(toggle);

    expect(screen.getByLabelText(/Reason for override/i)).toBeInTheDocument();

    // click toggle to close
    fireEvent.click(toggle);
    expect(screen.queryByLabelText(/Reason for override/i)).not.toBeInTheDocument();
  });

  // ---------------------------------------------------------------------------
  // Test: 8 — Override submit disabled when reason.length < 5
  // ---------------------------------------------------------------------------

  it("submit button is disabled when reason is fewer than 5 characters", () => {
    render(<GateVerdictBox {...makeProps({ verdict: "PARTIAL" })} />);

    fireEvent.click(screen.getByRole("button", { name: /Override: Mark done anyway/i }));

    const textarea = screen.getByLabelText(/Reason for override/i);
    fireEvent.change(textarea, { target: { value: "abc" } });

    const submitBtn = screen.getByRole("button", { name: /Mark Done — Override/i });
    expect(submitBtn).toBeDisabled();
    expect(submitBtn).toHaveAttribute("aria-disabled", "true");
  });

  // ---------------------------------------------------------------------------
  // Test: 9 — Override submit enabled when reason.length >= 5
  // ---------------------------------------------------------------------------

  it("submit button is enabled when reason has 5 or more characters", () => {
    render(<GateVerdictBox {...makeProps({ verdict: "PARTIAL" })} />);

    fireEvent.click(screen.getByRole("button", { name: /Override: Mark done anyway/i }));

    const textarea = screen.getByLabelText(/Reason for override/i);
    fireEvent.change(textarea, { target: { value: "valid reason" } });

    const submitBtn = screen.getByRole("button", { name: /Mark Done — Override/i });
    expect(submitBtn).not.toBeDisabled();
    expect(submitBtn).toHaveAttribute("aria-disabled", "false");
  });

  // ---------------------------------------------------------------------------
  // Test: 10 — Submitting override calls onOverride with the reason
  // ---------------------------------------------------------------------------

  it("clicking submit calls onOverride with the typed reason", async () => {
    const onOverride = jest.fn().mockResolvedValue(undefined);
    render(<GateVerdictBox {...makeProps({ verdict: "FAIL", onOverride })} />);

    fireEvent.click(screen.getByRole("button", { name: /Override: Mark done anyway/i }));

    const textarea = screen.getByLabelText(/Reason for override/i);
    fireEvent.change(textarea, { target: { value: "good enough reason" } });

    fireEvent.click(screen.getByRole("button", { name: /Mark Done — Override/i }));

    await waitFor(() => expect(onOverride).toHaveBeenCalledTimes(1));
    expect(onOverride).toHaveBeenCalledWith("good enough reason");
  });
});

// ---------------------------------------------------------------------------
// Test: 11 — Skip gate link shows confirmation
// ---------------------------------------------------------------------------

describe("GateVerdictBox — skip gate", () => {
  it("clicking the skip link shows the inline confirmation dialog", () => {
    render(<GateVerdictBox {...makeProps({ verdict: "PASS" })} />);

    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Skip gate and mark done without review/i }));

    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(screen.getByText(/Skip gate and mark done without review/i)).toBeInTheDocument();
  });

  // ---------------------------------------------------------------------------
  // Test: 12 — Cancel in confirmation hides it
  // ---------------------------------------------------------------------------

  it("clicking Cancel in confirmation hides the dialog", () => {
    render(<GateVerdictBox {...makeProps({ verdict: "PASS" })} />);

    fireEvent.click(screen.getByRole("button", { name: /Skip gate and mark done without review/i }));
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /^Cancel$/i }));
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  // ---------------------------------------------------------------------------
  // Test: 13 — Confirm button calls onSkipGate
  // ---------------------------------------------------------------------------

  it("clicking Confirm — Skip Gate calls onSkipGate", async () => {
    const onSkipGate = jest.fn().mockResolvedValue(undefined);
    render(<GateVerdictBox {...makeProps({ verdict: "PASS", onSkipGate })} />);

    fireEvent.click(screen.getByRole("button", { name: /Skip gate and mark done without review/i }));
    fireEvent.click(screen.getByRole("button", { name: /Confirm — Skip Gate/i }));

    await waitFor(() => expect(onSkipGate).toHaveBeenCalledTimes(1));
  });

  // ---------------------------------------------------------------------------
  // Test: 14 — Escape key in confirmation hides it
  // ---------------------------------------------------------------------------

  it("pressing Escape inside the confirmation dialog hides it", () => {
    render(<GateVerdictBox {...makeProps({ verdict: "PASS" })} />);

    fireEvent.click(screen.getByRole("button", { name: /Skip gate and mark done without review/i }));
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();

    const dialog = screen.getByRole("alertdialog");
    fireEvent.keyDown(dialog, { key: "Escape" });

    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  // ---------------------------------------------------------------------------
  // Test: 15 — Focus trap: Tab cycles between Cancel and Confirm only
  // ---------------------------------------------------------------------------

  it("GateVerdictBox_should_wrapFocusToCancelButton_When_TabPressedOnConfirmButton", () => {
    render(<GateVerdictBox {...makeProps({ verdict: "PASS" })} />);

    fireEvent.click(screen.getByRole("button", { name: /Skip gate and mark done without review/i }));

    const cancelBtn = screen.getByRole("button", { name: /^Cancel$/i });
    const confirmBtn = screen.getByRole("button", { name: /Confirm — Skip Gate/i });

    // useFocusTrap listens at the document level, not on the dialog element
    confirmBtn.focus();
    fireEvent.keyDown(document, { key: "Tab", shiftKey: false });
    expect(cancelBtn).toHaveFocus();
  });

  it("Shift+Tab in confirmation dialog wraps from Cancel back to Confirm", () => {
    render(<GateVerdictBox {...makeProps({ verdict: "PASS" })} />);

    fireEvent.click(screen.getByRole("button", { name: /Skip gate and mark done without review/i }));

    const cancelBtn = screen.getByRole("button", { name: /^Cancel$/i });
    const confirmBtn = screen.getByRole("button", { name: /Confirm — Skip Gate/i });

    cancelBtn.focus();
    fireEvent.keyDown(document, { key: "Tab", shiftKey: true });
    expect(confirmBtn).toHaveFocus();
  });

  it("GateVerdictBox_should_notThrow_When_ConfirmClickedAndTriggerButtonIsUnmounted", async () => {
    const onSkipGate = jest.fn().mockResolvedValue(undefined);
    render(<GateVerdictBox {...makeProps({ verdict: "PASS", onSkipGate })} />);

    fireEvent.click(screen.getByRole("button", { name: /Skip gate and mark done without review/i }));
    // The skip-link trigger button is unmounted while the dialog is open, so
    // useFocusTrap's cleanup (triggerEl?.focus()) has no live element to
    // restore focus to — this must not throw.
    expect(() => {
      fireEvent.click(screen.getByRole("button", { name: /Confirm — Skip Gate/i }));
    }).not.toThrow();

    await waitFor(() => expect(onSkipGate).toHaveBeenCalledTimes(1));
  });
});

// ---------------------------------------------------------------------------
// Test: 16 — Criteria list renders on PARTIAL with criteria
// ---------------------------------------------------------------------------

describe("GateVerdictBox — criteria list", () => {
  it("renders criteria when verdict is PARTIAL and criteria array is provided", () => {
    const criteria = [
      { label: "Unit tests pass", passed: true },
      { label: "Coverage >= 80%", passed: false },
    ];
    render(
      <GateVerdictBox
        {...makeProps({ verdict: "PARTIAL", criteria })}
      />
    );

    const list = screen.getByRole("list", { name: /Criteria results/i });
    expect(list).toBeInTheDocument();

    expect(screen.getByText("Unit tests pass")).toBeInTheDocument();
    expect(screen.getByText("Coverage >= 80%")).toBeInTheDocument();
  });

  it("renders criteria list for PASS verdict (shown for all verdicts)", () => {
    const criteria = [{ label: "Unit tests pass", passed: true }];
    render(
      <GateVerdictBox
        {...makeProps({ verdict: "PASS", criteria })}
      />
    );

    // feat(backlog): show review criteria for all verdicts
    expect(screen.getByRole("list", { name: /Criteria results/i })).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test: 17-19 — UNVERIFIABLE verdict
// ---------------------------------------------------------------------------

describe("GateVerdictBox — UNVERIFIABLE verdict", () => {
  it("renders Re-run Gate button when onReReview is provided and clicking it calls onReReview", async () => {
    const onReReview = jest.fn().mockResolvedValue(undefined);
    render(<GateVerdictBox {...makeProps({ verdict: "UNVERIFIABLE", onReReview })} />);

    expect(screen.getByText("UNVERIFIABLE")).toBeInTheDocument();

    const reReviewBtn = screen.getByRole("button", { name: /Re-run Gate/i });
    expect(reReviewBtn).toBeInTheDocument();

    fireEvent.click(reReviewBtn);

    await waitFor(() => expect(onReReview).toHaveBeenCalledTimes(1));
  });

  it("hides Re-run Gate button when onReReview is omitted", () => {
    render(<GateVerdictBox {...makeProps({ verdict: "UNVERIFIABLE", onReReview: undefined })} />);

    expect(screen.getByText("UNVERIFIABLE")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Re-run Gate/i })).not.toBeInTheDocument();
  });

  it("renders Reopen for Revision button and the Override section", () => {
    render(<GateVerdictBox {...makeProps({ verdict: "UNVERIFIABLE" })} />);

    expect(screen.getByRole("button", { name: /Reopen for Revision/i })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Override: Mark done anyway/i }),
    ).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Story 4.1.2: readOnly mode (Structured Diagnostic for Headless Diagnostic
// Sessions)
// ---------------------------------------------------------------------------

describe("GateVerdictBox_should_HideActionButtonRowButShowPerCriterionOutcomes_When_ReadOnlyIsTrueAndVerdictIsFail", () => {
  it("hides the entire action-button row but keeps verdict/summary/per-criterion outcomes", () => {
    const criteria = [
      { label: "AC1 — Passed", passed: true },
      { label: "AC2 — Failed: error not surfaced to user", passed: false },
    ];
    render(
      <GateVerdictBox
        verdict="FAIL"
        criteria={criteria}
        summary="2 of 5 criteria still not met."
        readOnly
      />
    );

    // Verdict card + per-criterion detail still present.
    expect(screen.getByText("FAILED")).toBeInTheDocument();
    expect(screen.getByText("2 of 5 criteria still not met.")).toBeInTheDocument();
    expect(screen.getByText("AC1 — Passed")).toBeInTheDocument();
    expect(screen.getByText("AC2 — Failed: error not surfaced to user")).toBeInTheDocument();

    // Approve/Reopen/Override/Skip Gate/Re-review row entirely absent.
    expect(screen.queryByRole("button", { name: /Approve/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Reopen for Revision/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/Override: Mark done anyway/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Skip gate and mark done without review/i)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Re-run Gate/i })).not.toBeInTheDocument();
  });

  it("ignores Ctrl+Enter in readOnly mode — no action buttons or forms appear", () => {
    // readOnly props have no write-mode callbacks at all (discriminated union),
    // so there is nothing to spy on here — the compiler enforces the "never
    // invoked" contract that this test used to check with a noop mock.
    render(<GateVerdictBox verdict="PASS" summary="All checks passed." readOnly />);

    const section = screen.getByRole("status", { name: /Gate verdict/i });
    fireEvent.keyDown(section, { key: "Enter", ctrlKey: true });

    expect(screen.queryByRole("button", { name: /Approve/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("form", { name: /Reopen for revision/i })).not.toBeInTheDocument();
  });
});
