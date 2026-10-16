/**
 * Story 6.1.1 / ux.md Surface 5: regex validation UX for the tagging-rule
 * pattern field — invalid pattern shows an aria-invalid/aria-describedby
 * associated inline error before any save round-trip; a valid edit clears it.
 */

import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TaggingRuleBuilderForm, PATTERN_ERROR_ID } from "./TaggingRuleBuilderForm";

describe("TaggingRuleBuilderForm", () => {
  const onSave = jest.fn().mockResolvedValue(undefined);
  const onCancel = jest.fn();

  beforeEach(() => jest.clearAllMocks());

  it("TaggingRuleBuilderForm_should_ShowAriaInvalidAndErrorText_When_PatternIsInvalidRegexOnBlur", () => {
    render(<TaggingRuleBuilderForm onSave={onSave} onCancel={onCancel} />);

    const patternInput = screen.getByTestId("tagging-rule-pattern-input");
    fireEvent.change(patternInput, { target: { value: "^(unterminated[" } });
    fireEvent.blur(patternInput);

    expect(patternInput.getAttribute("aria-invalid")).toBe("true");
    expect(patternInput.getAttribute("aria-describedby")).toBe(PATTERN_ERROR_ID);
    expect(screen.getByTestId("tagging-rule-pattern-error")).not.toBeNull();
  });

  it("TaggingRuleBuilderForm_should_ClearAriaInvalidAndUnmountError_When_PatternCorrectedAfterBlur", () => {
    render(<TaggingRuleBuilderForm onSave={onSave} onCancel={onCancel} />);

    const patternInput = screen.getByTestId("tagging-rule-pattern-input");
    fireEvent.change(patternInput, { target: { value: "^(unterminated[" } });
    fireEvent.blur(patternInput);
    expect(screen.getByTestId("tagging-rule-pattern-error")).not.toBeNull();

    fireEvent.change(patternInput, { target: { value: "^(fixed)/" } });

    expect(patternInput.getAttribute("aria-invalid")).toBeNull();
    expect(screen.queryByTestId("tagging-rule-pattern-error")).toBeNull();
  });

  it("TaggingRuleBuilderForm_should_KeepSaveEnabledAndFocusPatternField_When_SaveClickedWhileInvalid", async () => {
    render(<TaggingRuleBuilderForm onSave={onSave} onCancel={onCancel} />);

    fireEvent.change(screen.getByTestId("tagging-rule-name-input"), { target: { value: "Bad rule" } });
    fireEvent.change(screen.getByTestId("tagging-rule-output-tag-input"), { target: { value: "Bugfix" } });
    const patternInput = screen.getByTestId("tagging-rule-pattern-input");
    fireEvent.change(patternInput, { target: { value: "^(unterminated[" } });

    const saveButton = screen.getByRole("button", { name: "Save Rule" });
    expect(saveButton.hasAttribute("disabled")).toBe(false);
    fireEvent.click(saveButton);

    await waitFor(() => expect(screen.getByTestId("tagging-rule-pattern-error")).not.toBeNull());
    expect(onSave).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(patternInput);
  });

  it("TaggingRuleBuilderForm_should_Save_When_PatternIsValid", async () => {
    render(<TaggingRuleBuilderForm onSave={onSave} onCancel={onCancel} />);

    fireEvent.change(screen.getByTestId("tagging-rule-name-input"), { target: { value: "Bugfix branch" } });
    fireEvent.change(screen.getByTestId("tagging-rule-output-tag-input"), { target: { value: "Bugfix" } });
    fireEvent.change(screen.getByTestId("tagging-rule-pattern-input"), { target: { value: "^(bugfix|fix)/" } });

    fireEvent.click(screen.getByRole("button", { name: "Save Rule" }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    expect(onSave.mock.calls[0][0]).toMatchObject({
      name: "Bugfix branch",
      outputTag: "Bugfix",
      branchPattern: "^(bugfix|fix)/",
    });
  });
});
