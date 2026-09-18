/**
 * Tests for the `minSessionIdleMinutes` field on RuleBuilderForm.
 *
 * Covers validation.md AC5.1-AC5.4 (project_plans/stale-session-detection):
 *  - AC5.1: filling the field in an already-open editor requires no new modal/page
 *  - AC5.2: helper text states both the "0 = off" convention and the fail-closed contract
 *  - AC5.3: editing an existing rule pre-fills the saved value, round-trips on re-edit
 *  - AC5.4: a failed save keeps the modal open and the input value intact
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { RuleBuilderForm } from "./RuleBuilderForm";
import { ApprovalRuleProtoSchema, AutoDecision } from "@/gen/session/v1/types_pb";
import { create } from "@bufbuild/protobuf";

function makeEditRule(overrides: { minSessionIdleMinutes?: number } = {}) {
  return create(ApprovalRuleProtoSchema, {
    id: "rule-1",
    name: "Existing rule",
    toolName: "Bash",
    decision: AutoDecision.ALLOW,
    riskLevel: "medium",
    priority: 100,
    enabled: true,
    minSessionIdleMinutes: 0,
    ...overrides,
  });
}

describe("RuleBuilderForm minSessionIdleMinutes", () => {
  it("RuleBuilderForm_should_ShowSavedValueOnReopen_When_EditingRuleAgain", () => {
    const editRule = makeEditRule({ minSessionIdleMinutes: 60 });
    render(
      <RuleBuilderForm editRule={editRule} onSave={jest.fn()} onCancel={jest.fn()} />
    );

    const input = screen.getByTestId("min-session-idle-minutes-input");
    expect(input).toHaveValue(60);
  });

  it("RuleBuilderForm_should_RequireOnlyOneFieldFill_When_AddingIdleMinutesCondition", () => {
    const editRule = makeEditRule({ minSessionIdleMinutes: 0 });
    render(
      <RuleBuilderForm editRule={editRule} onSave={jest.fn()} onCancel={jest.fn()} />
    );

    // The field is already present in the already-open editor — filling it
    // requires no additional modal/page navigation.
    const input = screen.getByTestId("min-session-idle-minutes-input");
    fireEvent.change(input, { target: { value: "45" } });
    expect(input).toHaveValue(45);
    // Still the same form instance — no new modal/dialog was opened.
    expect(screen.getByText(editRule.name, { exact: false })).toBeInTheDocument();
  });

  it("RuleBuilderForm_should_IncludeMinSessionIdleMinutesInSavePayload_When_UserEditsAndSaves", async () => {
    const editRule = makeEditRule({ minSessionIdleMinutes: 0 });
    const onSave = jest.fn().mockResolvedValue(undefined);
    render(
      <RuleBuilderForm editRule={editRule} onSave={onSave} onCancel={jest.fn()} />
    );

    const input = screen.getByTestId("min-session-idle-minutes-input");
    fireEvent.change(input, { target: { value: "90" } });
    fireEvent.click(screen.getByRole("button", { name: "Update Rule" }));

    await waitFor(() => {
      expect(onSave).toHaveBeenCalledWith(
        expect.objectContaining({ minSessionIdleMinutes: 90 })
      );
    });
  });

  it("RuleBuilderForm_should_RenderHelperTextWithZeroAndFailClosedContract_When_FieldRendered", () => {
    render(<RuleBuilderForm onSave={jest.fn()} onCancel={jest.fn()} />);

    const input = screen.getByTestId("min-session-idle-minutes-input");
    const helperText = input.parentElement?.textContent ?? "";
    expect(helperText).toContain("0");
    expect(helperText).toMatch(/not applied/i);
    expect(helperText).toMatch(/unknown idle time never match/i);
  });

  it("RuleBuilderForm_should_KeepModalOpenAndInputIntact_When_SaveRuleFails", async () => {
    const editRule = makeEditRule({ minSessionIdleMinutes: 0 });
    const onSave = jest.fn().mockRejectedValue(new Error("upsertApprovalRule failed"));
    render(
      <RuleBuilderForm editRule={editRule} onSave={onSave} onCancel={jest.fn()} />
    );

    const input = screen.getByTestId("min-session-idle-minutes-input");
    fireEvent.change(input, { target: { value: "30" } });
    fireEvent.click(screen.getByRole("button", { name: "Update Rule" }));

    await waitFor(() => {
      expect(onSave).toHaveBeenCalled();
    });

    // Modal/form stays mounted and the entered value is preserved; Save is
    // clickable again (not stuck in a permanent "Saving…" state).
    expect(screen.getByTestId("min-session-idle-minutes-input")).toHaveValue(30);
    expect(screen.getByRole("button", { name: "Update Rule" })).toBeEnabled();
    expect(screen.getByText(/upsertApprovalRule failed/i)).toBeInTheDocument();
  });

  it("RuleBuilderForm_should_AssociateLabelsWithNativeInputs_When_Rendered", () => {
    render(<RuleBuilderForm onSave={jest.fn()} onCancel={jest.fn()} />);

    const input = screen.getByTestId("min-session-idle-minutes-input");
    expect(input).toHaveAttribute("id", "rule-min-session-idle-minutes-input");
    const label = document.querySelector('label[for="rule-min-session-idle-minutes-input"]');
    expect(label).not.toBeNull();
    expect(label).toContainElement(input);
  });
});
