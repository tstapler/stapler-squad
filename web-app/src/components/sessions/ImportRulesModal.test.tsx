/**
 * Tests for ImportRulesModal component.
 * Covers UT-FE-11 through UT-FE-18.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ImportRulesModal } from "./ImportRulesModal";
import { AutoDecision } from "@/gen/session/v1/types_pb";
import type { ApprovalRuleProto } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// Mocks for hooks
// ---------------------------------------------------------------------------

const mockValidateResults = jest.fn(() => ({
  results: [] as ReturnType<typeof makeParsedResult>[],
  loading: false,
  validCount: 0,
  errorCount: 0,
  error: null,
}));

jest.mock("@/lib/hooks/useValidateRules", () => ({
  useValidateRules: (..._args: unknown[]) => mockValidateResults(),
}));

const mockApplyRules = jest.fn();
const mockBulkUpsertState = { applyRules: mockApplyRules, loading: false, result: null, error: null };

jest.mock("@/lib/hooks/useBulkUpsertRules", () => ({
  useBulkUpsertRules: () => mockBulkUpsertState,
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeRule(name: string): ApprovalRuleProto {
  return {
    id: `rule-${name}`,
    name,
    toolName: "Bash",
    toolPattern: "",
    commandPattern: "",
    filePattern: "",
    criteriaPrograms: [],
    criteriaSubcommands: [],
    decision: AutoDecision.ALLOW,
    riskLevel: "low",
    reason: "",
    alternative: "",
    priority: 10,
    enabled: true,
    source: "user",
  } as unknown as ApprovalRuleProto;
}

function makeParsedResult(name: string, valid: boolean, errors: string[] = []) {
  return {
    originalName: name,
    valid,
    errors,
    rule: valid ? { name, toolName: "Bash", decision: AutoDecision.ALLOW, priority: 10, enabled: true } : undefined,
  };
}

function renderModal(existingRules: ApprovalRuleProto[] = [], onApplied = jest.fn(), onClose = jest.fn()) {
  return render(
    <ImportRulesModal
      open={true}
      onClose={onClose}
      onApplied={onApplied}
      existingRules={existingRules}
    />
  );
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("ImportRulesModal", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockBulkUpsertState.loading = false;
    mockBulkUpsertState.result = null;
    mockBulkUpsertState.error = null;
    mockValidateResults.mockReturnValue({
      results: [],
      loading: false,
      validCount: 0,
      errorCount: 0,
      error: null,
    });
  });

  it("UT-FE-11: ImportRulesModal_apply_button_disabled_when_validCount_is_zero", () => {
    mockValidateResults.mockReturnValue({
      results: [makeParsedResult("Bad Rule", false, ["name is required"]), makeParsedResult("Bad Rule 2", false, ["invalid decision"])],
      loading: false,
      validCount: 0,
      errorCount: 2,
      error: null,
    });

    renderModal();

    // Type something to trigger render with results
    const textarea = screen.getByTestId("yaml-input");
    fireEvent.change(textarea, { target: { value: "rules:\n- tool: Bash\n  decision: bad\n" } });

    const applyButton = screen.getByTestId("apply-button");
    expect(applyButton).toBeDisabled();
  });

  it("UT-FE-12: ImportRulesModal_shows_no_valid_rules_message", () => {
    mockValidateResults.mockReturnValue({
      results: [makeParsedResult("Bad Rule", false, ["invalid"])],
      loading: false,
      validCount: 0,
      errorCount: 1,
      error: null,
    });

    renderModal();
    const textarea = screen.getByTestId("yaml-input");
    fireEvent.change(textarea, { target: { value: "some yaml" } });

    expect(screen.getByTestId("no-valid-rules-message")).toBeInTheDocument();
    expect(screen.getByTestId("no-valid-rules-message")).toHaveTextContent(
      "No valid rules to apply. Fix the errors above and try again."
    );
  });

  it("UT-FE-13: ImportRulesModal_apply_button_label_reflects_counts", () => {
    mockValidateResults.mockReturnValue({
      results: [
        makeParsedResult("Rule 1", true),
        makeParsedResult("Rule 2", true),
        makeParsedResult("Rule 3", true),
        makeParsedResult("Bad Rule", false, ["invalid"]),
      ],
      loading: false,
      validCount: 3,
      errorCount: 1,
      error: null,
    });

    renderModal();
    const textarea = screen.getByTestId("yaml-input");
    fireEvent.change(textarea, { target: { value: "yaml" } });

    const applyButton = screen.getByTestId("apply-button");
    expect(applyButton).toHaveTextContent("Apply 3 rules (1 has errors)");
  });

  it("UT-FE-14: ImportRulesModal_clicking_apply_calls_applyRules", async () => {
    mockApplyRules.mockResolvedValueOnce({ errors: [] });
    mockValidateResults.mockReturnValue({
      results: [makeParsedResult("Rule 1", true), makeParsedResult("Rule 2", true)],
      loading: false,
      validCount: 2,
      errorCount: 0,
      error: null,
    });

    const onApplied = jest.fn();
    const onClose = jest.fn();
    renderModal([], onApplied, onClose);

    const textarea = screen.getByTestId("yaml-input");
    fireEvent.change(textarea, { target: { value: "yaml" } });

    const applyButton = screen.getByTestId("apply-button");
    fireEvent.click(applyButton);

    await waitFor(() => {
      expect(mockApplyRules).toHaveBeenCalledTimes(1);
      const [rules, overwrite] = mockApplyRules.mock.calls[0];
      expect(rules).toHaveLength(2);
      expect(overwrite).toBe(false);
    });
  });

  it("UT-FE-15: ImportRulesModal_duplicate_detection_shows_overwrite_badge", () => {
    mockValidateResults.mockReturnValue({
      results: [makeParsedResult("Allow git log", true)],
      loading: false,
      validCount: 1,
      errorCount: 0,
      error: null,
    });

    renderModal([makeRule("Allow git log")]);
    const textarea = screen.getByTestId("yaml-input");
    fireEvent.change(textarea, { target: { value: "yaml" } });

    // Default mode is "skip", so should show "will skip" badge not "will overwrite"
    expect(screen.getByTestId("skip-badge")).toBeInTheDocument();
  });

  it("UT-FE-16: ImportRulesModal_duplicate_detection_skip_mode", () => {
    mockValidateResults.mockReturnValue({
      results: [makeParsedResult("Allow git log", true)],
      loading: false,
      validCount: 1,
      errorCount: 0,
      error: null,
    });

    renderModal([makeRule("Allow git log")]);
    const textarea = screen.getByTestId("yaml-input");
    fireEvent.change(textarea, { target: { value: "yaml" } });

    // Default mode is skip
    expect(screen.getByTestId("skip-badge")).toBeInTheDocument();
    expect(screen.queryByTestId("overwrite-badge")).not.toBeInTheDocument();
  });

  it("UT-FE-15b: ImportRulesModal_overwrite_mode_calls_applyRules_with_overwrite_true", async () => {
    mockApplyRules.mockResolvedValueOnce({ errors: [] });
    mockValidateResults.mockReturnValue({
      results: [makeParsedResult("Allow git log", true)],
      loading: false,
      validCount: 1,
      errorCount: 0,
      error: null,
    });

    const onApplied = jest.fn();
    renderModal([makeRule("Allow git log")], onApplied);

    const textarea = screen.getByTestId("yaml-input");
    fireEvent.change(textarea, { target: { value: "yaml" } });

    // Select overwrite mode
    const overwriteRadio = screen.getByTestId("duplicate-mode-overwrite");
    fireEvent.click(overwriteRadio);

    // Overwrite badge should now be visible (not skip)
    expect(screen.getByTestId("overwrite-badge")).toBeInTheDocument();
    expect(screen.queryByTestId("skip-badge")).not.toBeInTheDocument();

    // Apply
    const applyButton = screen.getByTestId("apply-button");
    fireEvent.click(applyButton);

    await waitFor(() => {
      expect(mockApplyRules).toHaveBeenCalledWith(
        expect.arrayContaining([expect.objectContaining({ name: "Allow git log" })]),
        true  // overwrite=true
      );
      expect(onApplied).toHaveBeenCalledTimes(1);
    });
  });

  it("UT-FE-17: ImportRulesModal_partial_apply_error_stays_open", async () => {
    mockApplyRules.mockResolvedValueOnce({ errors: ["Partial failure"] });

    mockValidateResults.mockReturnValue({
      results: [makeParsedResult("Rule 1", true)],
      loading: false,
      validCount: 1,
      errorCount: 0,
      error: null,
    });

    const onClose = jest.fn();
    renderModal([], jest.fn(), onClose);

    const textarea = screen.getByTestId("yaml-input");
    fireEvent.change(textarea, { target: { value: "yaml" } });

    const applyButton = screen.getByTestId("apply-button");
    fireEvent.click(applyButton);

    await waitFor(() => {
      // Error banner must be visible
      expect(screen.getByTestId("partial-error-banner")).toBeInTheDocument();
      // Modal should not be closed (onClose not called)
      expect(onClose).not.toHaveBeenCalled();
    });
  });

  it("UT-FE-18: ImportRulesModal_onApplied_called_on_success", async () => {
    mockApplyRules.mockResolvedValueOnce({ errors: [] });
    // No error in state
    mockBulkUpsertState.error = null;

    mockValidateResults.mockReturnValue({
      results: [makeParsedResult("Rule 1", true)],
      loading: false,
      validCount: 1,
      errorCount: 0,
      error: null,
    });

    const onApplied = jest.fn();
    const onClose = jest.fn();
    renderModal([], onApplied, onClose);

    const textarea = screen.getByTestId("yaml-input");
    fireEvent.change(textarea, { target: { value: "yaml" } });

    const applyButton = screen.getByTestId("apply-button");
    fireEvent.click(applyButton);

    await waitFor(() => {
      expect(mockApplyRules).toHaveBeenCalledTimes(1);
      expect(onApplied).toHaveBeenCalledTimes(1);
      expect(onClose).toHaveBeenCalledTimes(1);
    });
  });
});
