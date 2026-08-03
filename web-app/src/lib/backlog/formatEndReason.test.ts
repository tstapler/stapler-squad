import { formatEndReason } from "./formatEndReason";

describe("formatEndReason", () => {
  it("formatEndReason_should_ReturnNoneSeverity_When_ReasonIsEmpty", () => {
    expect(formatEndReason("")).toEqual({ label: "", severity: "none" });
  });

  it("formatEndReason_should_ReturnNoneSeverity_When_ReasonIsShutdown", () => {
    expect(formatEndReason("shutdown")).toEqual({ label: "", severity: "none" });
  });

  it("formatEndReason_should_ReturnWarningSeverity_When_ReasonIsTimeout", () => {
    expect(formatEndReason("timeout")).toEqual({
      label: "Headless call timed out",
      severity: "warning",
    });
  });

  it("formatEndReason_should_ReturnWarningSeverity_When_ReasonIsProcessError", () => {
    expect(formatEndReason("process_error")).toEqual({
      label: "Headless call failed (process error)",
      severity: "warning",
    });
  });

  it("formatEndReason_should_ReturnErrorSeverity_When_ReasonIsClaudeNotFound", () => {
    expect(formatEndReason("claude_not_found")).toEqual({
      label: "Headless call failed — claude CLI not found",
      severity: "error",
    });
  });

  it("formatEndReason_should_ReturnErrorSeverity_When_ReasonIsOther", () => {
    expect(formatEndReason("other")).toEqual({
      label: "Headless call failed (unclassified)",
      severity: "error",
    });
  });

  // AC7: an unrecognized/future end_reason value must render a visible warning
  // chip, not silently render as clean success (severity "none") — the naive
  // default-to-none behavior is indistinguishable from a genuine clean exit,
  // which inverts the entire feature's purpose for the case that matters most.
  it("formatEndReason_should_ReturnWarningSeverityWithReasonInLabel_When_ReasonIsUnrecognized", () => {
    expect(formatEndReason("some_future_value")).toEqual({
      label: "Unrecognized end reason: some_future_value",
      severity: "warning",
    });
  });
});
