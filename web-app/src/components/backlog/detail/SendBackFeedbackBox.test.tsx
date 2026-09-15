/**
 * Tests for SendBackFeedbackBox (Story 2.1.1, plan.md Task 2.1.1c).
 *
 * Mirrors PlanVerdictBox.test.tsx's structure. Covers:
 *  1. Toggle reveals a role=form with a required textarea, Submit starts disabled
 *  2. Submit enables only once text is typed
 *  3. Escape cancels and returns focus to the toggle
 *  4. A resolved onSubmit clears the form
 *  5. InlineNotice renders only when activeWorkSessionCount > 0
 *  6. box renders nothing when visible is false
 *  7. The 5 distinct SendBackError-tagged failure branches (plain / transition /
 *     triage+ready / reject+ready / triage+idea)
 *  8. Retry behavior for the idea-landing case, incl. no-op on empty textarea
 *  9. Cancel/Escape are inert while a submit is in flight
 *  10. Mount-persistence: visible=false + actionError stays rendered; dismiss unmounts
 *  11. Retry-pending mount-persistence: never absent during a Retry's own pending window
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { SendBackFeedbackBox, type SendBackFeedbackBoxProps } from "./SendBackFeedbackBox";
import { SendBackError } from "./SendBackError";

function makeProps(overrides: Partial<SendBackFeedbackBoxProps> = {}): SendBackFeedbackBoxProps {
  return {
    visible: true,
    activeWorkSessionCount: 0,
    onSubmit: jest.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

/** Lets a test control exactly when an in-flight onSubmit call resolves/rejects. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function openFormAndType(text: string) {
  fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback"));
  fireEvent.change(screen.getByTestId("send-back-feedback-textarea"), {
    target: { value: text },
  });
}

describe("SendBackFeedbackBox — toggle + form basics", () => {
  test("toggle button reveals a role=form with a required textarea, and Submit starts disabled", () => {
    render(<SendBackFeedbackBox {...makeProps()} />);

    expect(screen.queryByRole("form")).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback"));

    const form = screen.getByRole("form", { name: "Send back for re-planning" });
    expect(form).toBeInTheDocument();
    expect(screen.getByLabelText("What should change? (required)")).toBeInTheDocument();

    const textarea = screen.getByTestId("send-back-feedback-textarea");
    expect(textarea).toBeInTheDocument();
    expect(textarea).toHaveAttribute("rows", "3");

    const submitBtn = screen.getByTestId("backlog-action-send-back-feedback-submit");
    expect(submitBtn).toBeDisabled();
    expect(submitBtn).toHaveAttribute("aria-disabled", "true");
  });

  test("box renders nothing when visible is false", () => {
    const { container } = render(<SendBackFeedbackBox {...makeProps({ visible: false })} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("submit is enabled only once non-empty text is typed", () => {
    render(<SendBackFeedbackBox {...makeProps()} />);
    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback"));

    const submitBtn = screen.getByTestId("backlog-action-send-back-feedback-submit");
    const textarea = screen.getByTestId("send-back-feedback-textarea");

    expect(submitBtn).toBeDisabled();

    fireEvent.change(textarea, { target: { value: "   " } });
    expect(submitBtn).toBeDisabled();
    expect(submitBtn).toHaveAttribute("aria-disabled", "true");

    fireEvent.change(textarea, {
      target: { value: "missed the mobile layout, redo with touch targets" },
    });
    expect(submitBtn).not.toBeDisabled();
    expect(submitBtn).toHaveAttribute("aria-disabled", "false");
  });

  it("Escape in the textarea cancels the form, clears text, and returns focus to the toggle", () => {
    render(<SendBackFeedbackBox {...makeProps()} />);
    openFormAndType("redo the auth approach");

    const textarea = screen.getByTestId("send-back-feedback-textarea");
    fireEvent.keyDown(textarea, { key: "Escape" });

    expect(screen.queryByTestId("send-back-feedback-textarea")).not.toBeInTheDocument();
    expect(screen.getByTestId("backlog-action-send-back-feedback")).toHaveFocus();
  });

  it("a resolved onSubmit clears the form", async () => {
    const onSubmit = jest.fn().mockResolvedValue(undefined);
    render(<SendBackFeedbackBox {...makeProps({ onSubmit })} />);
    openFormAndType("missed the mobile layout, redo with touch targets");
    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));

    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith("missed the mobile layout, redo with touch targets"));
    await waitFor(() => expect(screen.queryByTestId("send-back-feedback-textarea")).not.toBeInTheDocument());
  });
});

describe("SendBackFeedbackBox — active session notice", () => {
  it("renders the InlineNotice only when activeWorkSessionCount > 0", () => {
    const { rerender } = render(<SendBackFeedbackBox {...makeProps({ activeWorkSessionCount: 0 })} />);
    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback"));
    expect(screen.queryByTestId("send-back-active-session-notice")).not.toBeInTheDocument();

    rerender(<SendBackFeedbackBox {...makeProps({ activeWorkSessionCount: 1 })} />);
    expect(screen.getByTestId("send-back-active-session-notice")).toBeInTheDocument();
    expect(
      screen.getByText(
        "This item has an active session — submitting this will stop it, and its work may be superseded once re-planning completes."
      )
    ).toBeInTheDocument();

    // Its presence never disables Submit.
    fireEvent.change(screen.getByTestId("send-back-feedback-textarea"), {
      target: { value: "redo the auth approach" },
    });
    expect(screen.getByTestId("backlog-action-send-back-feedback-submit")).not.toBeDisabled();
  });
});

describe("SendBackFeedbackBox — failure branches", () => {
  it("a plain rejected onSubmit (no SendBackError) preserves typed text and shows the generic headline", async () => {
    const onSubmit = jest.fn().mockRejectedValue(new Error("network exploded"));
    render(<SendBackFeedbackBox {...makeProps({ onSubmit })} />);
    openFormAndType("redo the auth approach");
    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));

    await waitFor(() => expect(screen.getByText("Failed to send back")).toBeInTheDocument());
    expect(screen.getByText(/network exploded/)).toBeInTheDocument();
    expect(screen.getByTestId("send-back-feedback-textarea")).toHaveValue("redo the auth approach");
    expect(screen.getByTestId("backlog-action-send-back-feedback-submit")).not.toBeDisabled();
  });

  it('SendBackError("transition", ...) shows the same generic "Failed to send back" headline', async () => {
    const onSubmit = jest.fn().mockRejectedValue(new SendBackError("transition", "review", new Error("cas mismatch")));
    render(<SendBackFeedbackBox {...makeProps({ onSubmit })} />);
    openFormAndType("redo the auth approach");
    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));

    await waitFor(() => expect(screen.getByText("Failed to send back")).toBeInTheDocument());
    expect(screen.getByTestId("send-back-feedback-textarea")).toHaveValue("redo the auth approach");
  });

  it('SendBackError("triage", "ready", ...) points at "Regenerate Plan with This Feedback"', async () => {
    const onSubmit = jest.fn().mockRejectedValue(new SendBackError("triage", "ready", new Error("triage failed")));
    render(<SendBackFeedbackBox {...makeProps({ onSubmit })} />);
    openFormAndType("redo the auth approach");
    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));

    await waitFor(() => expect(screen.getByText("Sent back, but retriage didn't start")).toBeInTheDocument());
    expect(screen.getByText(/Regenerate Plan with This Feedback/)).toBeInTheDocument();
    expect(screen.queryByText(/Trigger Triage/)).not.toBeInTheDocument();
    // Not the retryable branch — no Retry affordance.
    expect(screen.queryByRole("button", { name: "Retry send-back with this feedback" })).not.toBeInTheDocument();
  });

  it('SendBackError("reject", "ready", ...) points at "Request Changes"', async () => {
    const onSubmit = jest.fn().mockRejectedValue(new SendBackError("reject", "ready", new Error("reject failed")));
    render(<SendBackFeedbackBox {...makeProps({ onSubmit })} />);
    openFormAndType("redo the auth approach");
    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));

    await waitFor(() => expect(screen.getByText("Sent back, but retriage didn't start")).toBeInTheDocument());
    expect(screen.getByText(/Request Changes/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Retry send-back with this feedback" })).not.toBeInTheDocument();
  });

  it('SendBackError("triage", "idea", ...) names "idea", offers no "will fail" claim, and renders Retry', async () => {
    const onSubmit = jest.fn().mockRejectedValue(new SendBackError("triage", "idea", new Error("triage failed")));
    render(<SendBackFeedbackBox {...makeProps({ onSubmit })} />);
    openFormAndType("redo the auth approach");
    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));

    await waitFor(() => expect(screen.getByText("Sent back, but retriage didn't start")).toBeInTheDocument());
    expect(screen.getByText(/moved back to "idea"/)).toBeInTheDocument();
    expect(screen.queryByText(/resubmitting.*will fail/i)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry send-back with this feedback" })).toBeInTheDocument();
    expect(screen.getByTestId("send-back-feedback-textarea")).toHaveValue("redo the auth approach");
  });
});

describe("SendBackFeedbackBox — Retry (idea-landing case)", () => {
  it("clicking Retry resubmits the same feedback, clears the error immediately, and a resolved retry behaves like a fresh success", async () => {
    const onSubmit = jest
      .fn()
      .mockRejectedValueOnce(new SendBackError("triage", "idea", new Error("triage failed")))
      .mockResolvedValueOnce(undefined);
    render(<SendBackFeedbackBox {...makeProps({ onSubmit })} />);
    openFormAndType("redo the auth approach");
    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));
    await waitFor(() => expect(screen.getByText("Sent back, but retriage didn't start")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Retry send-back with this feedback" }));

    // Error is cleared immediately, before the retry resolves.
    expect(screen.queryByText("Sent back, but retriage didn't start")).not.toBeInTheDocument();
    expect(onSubmit).toHaveBeenCalledTimes(2);
    expect(onSubmit).toHaveBeenLastCalledWith("redo the auth approach");

    await waitFor(() => expect(screen.queryByTestId("send-back-feedback-textarea")).not.toBeInTheDocument());
    expect(screen.queryByText("Sent back, but retriage didn't start")).not.toBeInTheDocument();
  });

  it("clicking Retry with an emptied textarea is a no-op", async () => {
    const onSubmit = jest.fn().mockRejectedValueOnce(new SendBackError("triage", "idea", new Error("triage failed")));
    render(<SendBackFeedbackBox {...makeProps({ onSubmit })} />);
    openFormAndType("redo the auth approach");
    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));
    await waitFor(() => expect(screen.getByText("Sent back, but retriage didn't start")).toBeInTheDocument());

    fireEvent.change(screen.getByTestId("send-back-feedback-textarea"), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "Retry send-back with this feedback" }));

    expect(onSubmit).toHaveBeenCalledTimes(1);
    // The stale error is untouched by the no-op retry.
    expect(screen.getByText("Sent back, but retriage didn't start")).toBeInTheDocument();
  });
});

describe("SendBackFeedbackBox — Cancel/Escape inert while pending", () => {
  it("Cancel is a no-op while isPending is true", async () => {
    const { promise, resolve } = deferred<void>();
    const onSubmit = jest.fn().mockReturnValue(promise);
    render(<SendBackFeedbackBox {...makeProps({ onSubmit })} />);
    openFormAndType("redo the auth approach");
    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));

    const submitBtn = screen.getByTestId("backlog-action-send-back-feedback-submit");
    expect(submitBtn).toHaveTextContent("Sending…");

    fireEvent.click(screen.getByRole("button", { name: /^Cancel$/i }));

    expect(screen.getByTestId("send-back-feedback-textarea")).toBeInTheDocument();
    expect(screen.getByTestId("send-back-feedback-textarea")).toHaveValue("redo the auth approach");
    expect(screen.getByTestId("backlog-action-send-back-feedback")).not.toHaveFocus();

    resolve();
    await waitFor(() => expect(screen.queryByTestId("send-back-feedback-textarea")).not.toBeInTheDocument());
  });

  it("Escape in the textarea is a no-op while isPending is true", async () => {
    const { promise, resolve } = deferred<void>();
    const onSubmit = jest.fn().mockReturnValue(promise);
    render(<SendBackFeedbackBox {...makeProps({ onSubmit })} />);
    openFormAndType("redo the auth approach");
    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));

    fireEvent.keyDown(screen.getByTestId("send-back-feedback-textarea"), { key: "Escape" });

    expect(screen.getByTestId("send-back-feedback-textarea")).toBeInTheDocument();
    expect(screen.getByTestId("send-back-feedback-textarea")).toHaveValue("redo the auth approach");

    resolve();
    await waitFor(() => expect(screen.queryByTestId("send-back-feedback-textarea")).not.toBeInTheDocument());
  });

  it("the Cancel button is disabled/aria-disabled while isPending", () => {
    const { promise } = deferred<void>();
    const onSubmit = jest.fn().mockReturnValue(promise);
    render(<SendBackFeedbackBox {...makeProps({ onSubmit })} />);
    openFormAndType("redo the auth approach");
    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));

    const cancelBtn = screen.getByRole("button", { name: /^Cancel$/i });
    expect(cancelBtn).toBeDisabled();
    expect(cancelBtn).toHaveAttribute("aria-disabled", "true");
  });
});

describe("SendBackFeedbackBox — mount persistence when visible flips false", () => {
  it("stays mounted showing the error UI when visible becomes false after a partial failure, and dismiss unmounts it cleanly", async () => {
    const onSubmit = jest.fn().mockRejectedValue(new SendBackError("triage", "ready", new Error("triage failed")));
    const { rerender } = render(<SendBackFeedbackBox {...makeProps({ visible: true, onSubmit })} />);
    openFormAndType("redo the auth approach");
    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));
    await waitFor(() => expect(screen.getByText("Sent back, but retriage didn't start")).toBeInTheDocument());

    // Simulates the parent's post-failure load() flipping visible false.
    rerender(<SendBackFeedbackBox {...makeProps({ visible: false, onSubmit })} />);

    expect(screen.getByText("Sent back, but retriage didn't start")).toBeInTheDocument();
    expect(screen.queryByTestId("backlog-action-send-back-feedback")).not.toBeInTheDocument();

    fireEvent.click(screen.getByLabelText("Dismiss error"));

    expect(screen.queryByText("Sent back, but retriage didn't start")).not.toBeInTheDocument();
  });

  it("renders null immediately when visible=false and there is no actionError", () => {
    const { container } = render(<SendBackFeedbackBox {...makeProps({ visible: false })} />);
    expect(container).toBeEmptyDOMElement();
  });
});

describe("SendBackFeedbackBox — retry-pending mount persistence", () => {
  it("is never absent during a Retry's own pending window, even from visible=false", async () => {
    const firstAttempt = deferred<void>();
    const onSubmit = jest.fn().mockReturnValueOnce(firstAttempt.promise);
    const { rerender } = render(<SendBackFeedbackBox {...makeProps({ visible: true, onSubmit })} />);
    openFormAndType("redo the auth approach");
    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));

    firstAttempt.reject(new SendBackError("triage", "idea", new Error("triage failed")));
    await waitFor(() => expect(screen.getByText(/moved back to "idea"/)).toBeInTheDocument());

    // Parent's load() has already moved the item off the eligible set.
    rerender(<SendBackFeedbackBox {...makeProps({ visible: false, onSubmit })} />);
    expect(screen.getByText(/moved back to "idea"/)).toBeInTheDocument();

    const secondAttempt = deferred<void>();
    onSubmit.mockReturnValueOnce(secondAttempt.promise);

    fireEvent.click(screen.getByRole("button", { name: "Retry send-back with this feedback" }));

    // Mid-retry: visible=false, actionError=null, isPending=true — the
    // component must not disappear for this window (iteration-4 BLOCKER fix).
    expect(screen.queryByText(/moved back to "idea"/)).not.toBeInTheDocument();
    const submitBtn = screen.getByTestId("backlog-action-send-back-feedback-submit");
    expect(submitBtn).toBeInTheDocument();
    expect(submitBtn).toHaveTextContent("Sending…");
    expect(submitBtn).toHaveAttribute("aria-busy", "true");
    expect(screen.getByTestId("send-back-feedback-textarea")).toHaveValue("redo the auth approach");
    expect(onSubmit).toHaveBeenLastCalledWith("redo the auth approach");

    secondAttempt.resolve();
    await waitFor(() => expect(screen.queryByTestId("send-back-feedback-textarea")).not.toBeInTheDocument());
    expect(screen.queryByText(/moved back to "idea"/)).not.toBeInTheDocument();
  });

  it("on a second rejection, shows the appropriate error again and remains mounted", async () => {
    const firstAttempt = deferred<void>();
    const onSubmit = jest.fn().mockReturnValueOnce(firstAttempt.promise);
    const { rerender } = render(<SendBackFeedbackBox {...makeProps({ visible: true, onSubmit })} />);
    openFormAndType("redo the auth approach");
    fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));

    firstAttempt.reject(new SendBackError("triage", "idea", new Error("triage failed")));
    await waitFor(() => expect(screen.getByText(/moved back to "idea"/)).toBeInTheDocument());

    rerender(<SendBackFeedbackBox {...makeProps({ visible: false, onSubmit })} />);

    const secondAttempt = deferred<void>();
    onSubmit.mockReturnValueOnce(secondAttempt.promise);
    fireEvent.click(screen.getByRole("button", { name: "Retry send-back with this feedback" }));

    secondAttempt.reject(new SendBackError("triage", "idea", new Error("triage failed again")));

    await waitFor(() => expect(screen.getByText(/moved back to "idea"/)).toBeInTheDocument());
    expect(screen.getByTestId("send-back-feedback-textarea")).toHaveValue("redo the auth approach");
    expect(screen.getByRole("button", { name: "Retry send-back with this feedback" })).toBeInTheDocument();
  });
});
