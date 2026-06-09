/**
 * Tests for ApprovalRulesPanel — Epic 3, Epic 6, and YAML import/export.
 *
 * Covers:
 *  - ApprovalRulesPanel_should_showGenerateSuggestionsButton
 *  - ApprovalRulesPanel_should_showSuggestedRuleCards_When_SuggestionsReturned
 *  - ApprovalRulesPanel_should_prefillForm_When_CommandSampleSuggestionReturns
 *  - UT-FE-23 through UT-FE-27 (YAML import/export + UX improvements)
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ApprovalRulesPanel } from "./ApprovalRulesPanel";
import { AutoDecision, SuggestionSource } from "@/gen/session/v1/types_pb";
import type { SuggestedRuleProto } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockRefresh = jest.fn();
const mockUpsertRule = jest.fn();
const mockDeleteRule = jest.fn();

jest.mock("@/lib/hooks/useApprovalRules", () => ({
  useApprovalRules: () => ({
    rules: [],
    loading: false,
    error: null,
    upsertRule: mockUpsertRule,
    deleteRule: mockDeleteRule,
    refresh: mockRefresh,
  }),
}));

jest.mock("@/lib/hooks/useApprovalAnalytics", () => ({
  useApprovalAnalytics: () => ({
    summary: null,
    loading: false,
    error: null,
  }),
}));

const mockExportRules = jest.fn();
jest.mock("@/lib/hooks/useExportRules", () => ({
  useExportRules: () => ({
    exportRules: mockExportRules,
    loading: false,
    error: null,
  }),
}));

// Stub ImportRulesModal so it renders a lightweight sentinel instead of the full modal.
jest.mock("./ImportRulesModal", () => ({
  ImportRulesModal: ({ open, onClose }: { open: boolean; onClose: () => void }) =>
    open ? (
      <div data-testid="import-rules-modal">
        <button data-testid="close-import-modal" onClick={onClose}>Close</button>
      </div>
    ) : null,
}));

// Control the two useGenerateRule instances via a shared config object.
// The first hook instance is "panel", the second is "cmd".
// We use a simple round-robin via a counter that resets each render cycle.
interface HookConfig {
  panel: {
    suggestions: SuggestedRuleProto[];
    loading: boolean;
    error: Error | null;
    generate: jest.Mock;
    cancel: jest.Mock;
    clear: jest.Mock;
  };
  cmd: {
    suggestions: SuggestedRuleProto[];
    loading: boolean;
    error: Error | null;
    generate: jest.Mock;
    cancel: jest.Mock;
    clear: jest.Mock;
  };
}

const hookConfig: HookConfig = {
  panel: {
    suggestions: [],
    loading: false,
    error: null,
    generate: jest.fn().mockResolvedValue(undefined),
    cancel: jest.fn(),
    clear: jest.fn(),
  },
  cmd: {
    suggestions: [],
    loading: false,
    error: null,
    generate: jest.fn().mockResolvedValue(undefined),
    cancel: jest.fn(),
    clear: jest.fn(),
  },
};

// The component calls useGenerateRule() twice per render:
// first call → panel instance, second call → cmd instance.
// We use an explicit call-count tracker that resets each test in resetHookConfig().
// Using call count (not index modulo) keeps the assignment deterministic even if
// React invokes extra renders in strict/concurrent mode — both renders see the
// same panel/cmd split because we reset before each test.
let _callCount = 0;

jest.mock("@/lib/hooks/useGenerateRule", () => ({
  useGenerateRule: () => {
    const callInRender = _callCount++ % 2;
    if (callInRender === 0) {
      return { ...hookConfig.panel };
    }
    return { ...hookConfig.cmd };
  },
}));

// SuggestedRuleCard mocked to a minimal stub.
jest.mock("./SuggestedRuleCard", () => ({
  SuggestedRuleCard: ({
    suggestion,
    onAccept,
    onDiscard,
  }: {
    suggestion: { name: string };
    onAccept: (r: unknown) => void;
    onDiscard: () => void;
  }) => (
    <div data-testid="suggested-rule-card">
      <span data-testid="suggestion-name">{suggestion.name}</span>
      <button data-testid="card-accept" onClick={() => onAccept({})}>Accept</button>
      <button data-testid="card-discard" onClick={onDiscard}>Discard</button>
    </div>
  ),
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeSuggestion(
  name = "Allow git status",
  overrides: Partial<Record<string, unknown>> = {}
): SuggestedRuleProto {
  return {
    name,
    toolName: "Bash",
    toolPattern: "",
    commandPattern: "^git status",
    filePattern: "",
    decision: AutoDecision.ALLOW,
    riskLevel: "low",
    reason: "Read-only command",
    alternative: "",
    priority: 100,
    confidence: 0.9,
    explanation: "Safe read-only operation",
    sourceCommands: [],
    shadowedByRuleIds: [],
    shadowsRuleIds: [],
    ...overrides,
  } as unknown as SuggestedRuleProto;
}

function resetHookConfig() {
  _callCount = 0;

  hookConfig.panel.suggestions = [];
  hookConfig.panel.loading = false;
  hookConfig.panel.error = null;
  hookConfig.panel.generate = jest.fn().mockResolvedValue(undefined);
  hookConfig.panel.cancel = jest.fn();
  hookConfig.panel.clear = jest.fn();

  hookConfig.cmd.suggestions = [];
  hookConfig.cmd.loading = false;
  hookConfig.cmd.error = null;
  hookConfig.cmd.generate = jest.fn().mockResolvedValue(undefined);
  hookConfig.cmd.cancel = jest.fn();
  hookConfig.cmd.clear = jest.fn();

  mockRefresh.mockClear();
  mockUpsertRule.mockClear();
  mockDeleteRule.mockClear();
  mockExportRules.mockClear();
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("ApprovalRulesPanel", () => {
  beforeEach(() => {
    resetHookConfig();
  });

  // ── Epic 3: Generate Suggestions button ────────────────────────────────────

  describe("Generate Suggestions button", () => {
    it("ApprovalRulesPanel_should_showGenerateSuggestionsButton", () => {
      render(<ApprovalRulesPanel />);

      const btn = screen.getByTestId("generate-suggestions");
      expect(btn).toBeInTheDocument();
      expect(btn).toHaveTextContent("Generate Suggestions");
      expect(btn).not.toBeDisabled();
    });

    it("shows Generating… label and Cancel button while loading", () => {
      hookConfig.panel.loading = true;
      render(<ApprovalRulesPanel />);

      const btn = screen.getByTestId("generate-suggestions");
      expect(btn).toHaveTextContent("Generating…");
      expect(btn).toBeDisabled();

      const cancelBtn = screen.getByTestId("cancel-generate-button");
      expect(cancelBtn).toBeInTheDocument();
    });

    it("calls generate with ANALYTICS_GAPS source when button clicked", async () => {
      render(<ApprovalRulesPanel />);

      fireEvent.click(screen.getByTestId("generate-suggestions"));

      await waitFor(() => expect(hookConfig.panel.generate).toHaveBeenCalledTimes(1));
      expect(hookConfig.panel.generate).toHaveBeenCalledWith(
        expect.objectContaining({ source: SuggestionSource.ANALYTICS_GAPS })
      );
    });

    it("calls cancel() when Cancel button is clicked during loading", () => {
      hookConfig.panel.loading = true;
      render(<ApprovalRulesPanel />);

      fireEvent.click(screen.getByTestId("cancel-generate-button"));
      expect(hookConfig.panel.cancel).toHaveBeenCalledTimes(1);
    });

    it("shows error banner when generate error is non-null", () => {
      hookConfig.panel.error = new Error("AI service unavailable");
      render(<ApprovalRulesPanel />);

      const banner = screen.getByTestId("generate-error-banner");
      expect(banner).toBeInTheDocument();
      expect(banner).toHaveTextContent("AI service unavailable");
    });

    it("calls clear() when dismiss error button is clicked", () => {
      hookConfig.panel.error = new Error("something failed");
      render(<ApprovalRulesPanel />);

      fireEvent.click(screen.getByTestId("dismiss-error-button"));
      expect(hookConfig.panel.clear).toHaveBeenCalledTimes(1);
    });
  });

  // ── Epic 3: Suggestion cards ───────────────────────────────────────────────

  describe("ApprovalRulesPanel_should_showSuggestedRuleCards_When_SuggestionsReturned", () => {
    it("renders one SuggestedRuleCard per suggestion", () => {
      hookConfig.panel.suggestions = [
        makeSuggestion("Rule A"),
        makeSuggestion("Rule B"),
      ];
      render(<ApprovalRulesPanel />);

      const cards = screen.getAllByTestId("suggested-rule-card");
      expect(cards).toHaveLength(2);
      expect(screen.getByText("Rule A")).toBeInTheDocument();
      expect(screen.getByText("Rule B")).toBeInTheDocument();
    });

    it("hides a card when its Discard button is clicked", () => {
      hookConfig.panel.suggestions = [makeSuggestion("Rule A"), makeSuggestion("Rule B")];
      render(<ApprovalRulesPanel />);

      // Discard the first card.
      const discardBtns = screen.getAllByTestId("card-discard");
      fireEvent.click(discardBtns[0]);

      // Only one card remains.
      expect(screen.getAllByTestId("suggested-rule-card")).toHaveLength(1);
      expect(screen.queryByText("Rule A")).not.toBeInTheDocument();
      expect(screen.getByText("Rule B")).toBeInTheDocument();
    });

    it("calls clear() and refresh() when Accept is clicked on a card", async () => {
      hookConfig.panel.suggestions = [makeSuggestion("Rule A")];
      render(<ApprovalRulesPanel />);

      fireEvent.click(screen.getByTestId("card-accept"));

      await waitFor(() => {
        expect(hookConfig.panel.clear).toHaveBeenCalledTimes(1);
        expect(mockRefresh).toHaveBeenCalledTimes(1);
      });
    });
  });

  // ── Epic 6: Command sample pre-fill ───────────────────────────────────────

  describe("ApprovalRulesPanel_should_prefillForm_When_CommandSampleSuggestionReturns", () => {
    it("shows Generate from command section when form is open", () => {
      render(<ApprovalRulesPanel />);

      // Open the form.
      fireEvent.click(screen.getByTestId("add-rule-button"));

      const details = screen.getByTestId("generate-from-command-details");
      expect(details).toBeInTheDocument();

      const textarea = screen.getByTestId("command-sample-textarea");
      expect(textarea).toBeInTheDocument();
    });

    it("Generate button is disabled when textarea is empty", () => {
      render(<ApprovalRulesPanel />);
      fireEvent.click(screen.getByTestId("add-rule-button"));

      const genBtn = screen.getByTestId("command-sample-generate-button");
      expect(genBtn).toBeDisabled();
    });

    it("Generate button is enabled when textarea has content", () => {
      render(<ApprovalRulesPanel />);
      fireEvent.click(screen.getByTestId("add-rule-button"));

      const textarea = screen.getByTestId("command-sample-textarea");
      fireEvent.change(textarea, { target: { value: "git push origin main" } });

      const genBtn = screen.getByTestId("command-sample-generate-button");
      expect(genBtn).not.toBeDisabled();
    });

    it("calls cmdGenerate with COMMAND_SAMPLE source when Generate clicked", async () => {
      render(<ApprovalRulesPanel />);
      fireEvent.click(screen.getByTestId("add-rule-button"));

      const textarea = screen.getByTestId("command-sample-textarea");
      fireEvent.change(textarea, { target: { value: "git push origin main" } });

      fireEvent.click(screen.getByTestId("command-sample-generate-button"));

      await waitFor(() => expect(hookConfig.cmd.generate).toHaveBeenCalledTimes(1));
      expect(hookConfig.cmd.generate).toHaveBeenCalledWith(
        expect.objectContaining({
          source: SuggestionSource.COMMAND_SAMPLE,
          commandSample: "git push origin main",
        })
      );
    });

    it("pre-fills commandPattern from suggestion and shows AI badge", () => {
      // Set cmd suggestions before render so they are visible on mount.
      hookConfig.cmd.suggestions = [
        makeSuggestion("AI Rule", { commandPattern: "^git push" }),
      ];

      render(<ApprovalRulesPanel />);
      fireEvent.click(screen.getByTestId("add-rule-button"));

      // The component should have pre-filled the commandPattern field.
      const cmdInput = screen.getByTestId("form-command-pattern-input") as HTMLInputElement;
      expect(cmdInput.value).toBe("^git push");

      // AI badge should be visible.
      expect(screen.getByTestId("ai-generated-badge")).toBeInTheDocument();
    });

    it("does not overwrite a user-edited field with AI suggestion on re-render", () => {
      // No suggestions on initial render.
      render(<ApprovalRulesPanel />);
      fireEvent.click(screen.getByTestId("add-rule-button"));

      // User manually edits commandPattern BEFORE any AI suggestion arrives.
      const cmdInput = screen.getByTestId("form-command-pattern-input");
      fireEvent.change(cmdInput, { target: { value: "^my-custom-pattern" } });

      // The user-typed value should still be present.
      expect((screen.getByTestId("form-command-pattern-input") as HTMLInputElement).value).toBe(
        "^my-custom-pattern"
      );
    });

    it("resets form and calls cmdGenClear when form is closed and reopened", () => {
      render(<ApprovalRulesPanel />);

      // Open.
      fireEvent.click(screen.getByTestId("add-rule-button"));
      // Close.
      fireEvent.click(screen.getByText("Cancel"));

      expect(hookConfig.cmd.clear).toHaveBeenCalled();

      // Reopen — form should be clean.
      fireEvent.click(screen.getByTestId("add-rule-button"));
      const nameInput = screen.getByTestId("form-name-input") as HTMLInputElement;
      expect(nameInput.value).toBe("");
    });
  });

  // ── URL param pre-fill (BLOCKER) ──────────────────────────────────────────

  describe("URL param pre-fill", () => {
    afterEach(() => {
      window.history.pushState({}, "", "/");
    });

    it("opens modal and pre-fills name + toolName when ?tool param present", () => {
      window.history.pushState({}, "", "?tool=Bash");
      render(<ApprovalRulesPanel />);
      expect(screen.getByRole("dialog")).toBeInTheDocument();
      expect((screen.getByTestId("form-name-input") as HTMLInputElement).value).toBe("Allow Bash");
    });

    it("pre-fills criteriaPrograms and name when ?program param present", () => {
      window.history.pushState({}, "", "?program=git");
      render(<ApprovalRulesPanel />);
      expect(screen.getByRole("dialog")).toBeInTheDocument();
      expect((screen.getByTestId("form-name-input") as HTMLInputElement).value).toBe("Allow git");
      expect((screen.getByTestId("form-criteria-programs-input") as HTMLInputElement).value).toBe("git");
    });

    it("includes subcommand in name and subcommands when ?program+?subcommand present", () => {
      window.history.pushState({}, "", "?program=git&subcommand=push");
      render(<ApprovalRulesPanel />);
      expect((screen.getByTestId("form-name-input") as HTMLInputElement).value).toBe("Allow git push");
      expect((screen.getByTestId("form-criteria-subcommands-input") as HTMLInputElement).value).toBe("push");
    });

    it("does not open modal when no known params present", () => {
      window.history.pushState({}, "", "?unrelated=x");
      render(<ApprovalRulesPanel />);
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });

    it("opens modal blank when only ?open param present", () => {
      window.history.pushState({}, "", "?open=1");
      render(<ApprovalRulesPanel />);
      expect(screen.getByRole("dialog")).toBeInTheDocument();
      expect((screen.getByTestId("form-name-input") as HTMLInputElement).value).toBe("");
    });
  });

  // ── Modal open/close state ────────────────────────────────────────────────

  describe("Modal open/close", () => {
    it("dialog is not rendered on initial load", () => {
      render(<ApprovalRulesPanel />);
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });

    it("dialog opens when add-rule-button is clicked", () => {
      render(<ApprovalRulesPanel />);
      fireEvent.click(screen.getByTestId("add-rule-button"));
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });

    it("dialog closes when Cancel is clicked", () => {
      render(<ApprovalRulesPanel />);
      fireEvent.click(screen.getByTestId("add-rule-button"));
      fireEvent.click(screen.getByText("Cancel"));
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
  });

  // ── Modal dismiss paths (Escape key, × button) ────────────────────────────

  describe("Modal dismiss paths", () => {
    it("closes dialog when Escape is pressed", async () => {
      render(<ApprovalRulesPanel />);
      fireEvent.click(screen.getByTestId("add-rule-button"));
      expect(screen.getByRole("dialog")).toBeInTheDocument();
      fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape", code: "Escape" });
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    });

    it("closes dialog when × button is clicked", async () => {
      render(<ApprovalRulesPanel />);
      fireEvent.click(screen.getByTestId("add-rule-button"));
      fireEvent.click(screen.getByRole("button", { name: "Close dialog" }));
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    });
  });

  // ── Empty state copy ──────────────────────────────────────────────────────

  describe("Empty state", () => {
    it("UT-FE-23: ApprovalRulesPanel_empty_state_explains_purpose", () => {
      render(<ApprovalRulesPanel />);
      const emptyState = screen.getByTestId("empty-state");
      expect(emptyState).toBeInTheDocument();
      expect(emptyState).toHaveTextContent(/Approval rules let you automatically/i);
    });

    it("UT-FE-24: ApprovalRulesPanel_empty_state_seed_no_cta", () => {
      render(<ApprovalRulesPanel />);
      // Switch to the "Built-in" (seed) filter tab.
      fireEvent.click(screen.getByRole("button", { name: /Built-in/ }));
      const emptyState = screen.getByTestId("empty-state");
      expect(emptyState).toBeInTheDocument();
      // No "+ Add Rule" CTA inside the empty state for seed filter.
      // (The header button is always present but not within the empty-state div.)
      const { queryByText } = { queryByText: (text: RegExp) => emptyState.textContent?.match(text) };
      expect(queryByText(/\+ Add Rule/i)).toBeNull();
    });

    it("shows add-rule and import-yaml links in empty state for 'all' source filter", () => {
      render(<ApprovalRulesPanel />);
      const emptyState = screen.getByTestId("empty-state");
      expect(emptyState).toHaveTextContent(/Add Rule/);
      expect(emptyState).toHaveTextContent(/Import YAML/);
    });
  });

  // ── YAML import/export (UT-FE-25 through UT-FE-27) ───────────────────────

  describe("YAML import/export", () => {
    it("UT-FE-25: ApprovalRulesPanel_import_yaml_button_opens_modal", async () => {
      render(<ApprovalRulesPanel />);

      // The modal should not be mounted yet.
      expect(screen.queryByTestId("import-rules-modal")).not.toBeInTheDocument();

      // Click the Import YAML header button.
      fireEvent.click(screen.getByTestId("import-yaml-button"));

      await waitFor(() => {
        expect(screen.getByTestId("import-rules-modal")).toBeInTheDocument();
      });
    });

    it("UT-FE-26: ApprovalRulesPanel_export_yaml_button_triggers_hook", () => {
      render(<ApprovalRulesPanel />);

      fireEvent.click(screen.getByTestId("export-yaml-button"));

      expect(mockExportRules).toHaveBeenCalledTimes(1);
    });

    it("UT-FE-27: ApprovalRulesPanel_field_order_regex_after_separator", () => {
      render(<ApprovalRulesPanel />);
      fireEvent.click(screen.getByTestId("add-rule-button"));

      // The form should be open.
      const form = screen.getByRole("dialog");
      expect(form).toBeInTheDocument();

      // The separator should be present.
      const separator = screen.getByTestId("advanced-regex-separator");
      expect(separator).toBeInTheDocument();

      // Tool Name field should appear before the separator.
      const toolNameInput = screen.getByTestId("form-tool-name-input");
      const commandPatternInput = screen.getByTestId("form-command-pattern-input");
      expect(toolNameInput).toBeInTheDocument();
      expect(commandPatternInput).toBeInTheDocument();

      // Assert DOM order: toolName precedes separator precedes commandPattern.
      const allInputs = form.querySelectorAll("[data-testid]");
      const ids = Array.from(allInputs).map((el) => el.getAttribute("data-testid"));
      const toolNameIdx = ids.indexOf("form-tool-name-input");
      const sepIdx = ids.indexOf("advanced-regex-separator");
      const cmdPatternIdx = ids.indexOf("form-command-pattern-input");
      expect(toolNameIdx).toBeLessThan(sepIdx);
      expect(sepIdx).toBeLessThan(cmdPatternIdx);
    });
  });
});
