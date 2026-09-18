import { composeQuestionAnswerFeedback } from "./composeQuestionAnswerFeedback";

describe("composeQuestionAnswerFeedback", () => {
  it("composes a Q:/A: string from the rendered question text and the operator's answer", () => {
    expect(
      composeQuestionAnswerFeedback(
        "Should retries be per-workflow or global?",
        "Per-workflow, default to global"
      )
    ).toBe("Q: Should retries be per-workflow or global?\nA: Per-workflow, default to global");
  });

  it("trims leading/trailing whitespace on both the question and the answer", () => {
    expect(composeQuestionAnswerFeedback("  Padded question?  ", "  padded answer  ")).toBe(
      "Q: Padded question?\nA: padded answer"
    );
  });

  it("preserves internal newlines in a multi-line answer without corrupting the Q:/A: prefixes", () => {
    expect(
      composeQuestionAnswerFeedback("Single question?", "Line one\nLine two\nLine three")
    ).toBe("Q: Single question?\nA: Line one\nLine two\nLine three");
  });
});
