/**
 * Branch-coverage tests for deriveSendBackErrorCopy — the pure decision
 * matrix extracted from SendBackFeedbackBox's handleSubmit catch block.
 * SendBackFeedbackBox.test.tsx keeps only the DOM-level tests that this
 * copy actually renders and wires up Retry correctly; the branch logic
 * itself belongs here.
 */

import { deriveSendBackErrorCopy, SendBackError } from "./SendBackError";

describe("deriveSendBackErrorCopy", () => {
  it("falls back to the generic headline for a plain (non-SendBackError) rejection", () => {
    const copy = deriveSendBackErrorCopy(new Error("network exploded"));
    expect(copy).toEqual({ headline: "Failed to send back", message: "network exploded" });
  });

  it('uses the generic headline for failedAt="transition" — nothing changed server-side', () => {
    const copy = deriveSendBackErrorCopy(new SendBackError("transition", "review", new Error("cas mismatch")));
    expect(copy.headline).toBe("Failed to send back");
    expect(copy.retryable).toBeUndefined();
  });

  it('failedAt="triage" landing at "ready" points at "Regenerate Plan with This Feedback" and is not retryable', () => {
    const copy = deriveSendBackErrorCopy(new SendBackError("triage", "ready", new Error("triage failed")));
    expect(copy.headline).toBe("Sent back, but retriage didn't start");
    expect(copy.message).toMatch(/Regenerate Plan with This Feedback/);
    expect(copy.message).not.toMatch(/Trigger Triage/);
    expect(copy.retryable).toBeUndefined();
  });

  it('failedAt="reject" points at "Request Changes" and is not retryable', () => {
    const copy = deriveSendBackErrorCopy(new SendBackError("reject", "ready", new Error("reject failed")));
    expect(copy.headline).toBe("Sent back, but retriage didn't start");
    expect(copy.message).toMatch(/Request Changes/);
    expect(copy.retryable).toBeUndefined();
  });

  it('itemStatusAfterFailure="idea" (any failedAt) names "idea", makes no "will fail" claim, and is retryable', () => {
    const copy = deriveSendBackErrorCopy(new SendBackError("triage", "idea", new Error("triage failed")));
    expect(copy.headline).toBe("Sent back, but retriage didn't start");
    expect(copy.message).toMatch(/moved back to "idea"/);
    expect(copy.message).not.toMatch(/resubmitting.*will fail/i);
    expect(copy.retryable).toBe(true);
  });
});
