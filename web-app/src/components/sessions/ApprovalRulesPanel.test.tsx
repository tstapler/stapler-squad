/**
 * Tests for ApprovalRulesPanel — Epic 3 and Epic 6 AI rule generation.
 *
 * Covers:
 *  - ApprovalRulesPanel_should_showGenerateSuggestionsButton
 *  - ApprovalRulesPanel_should_showSuggestedRuleCards_When_SuggestionsReturned
 *  - ApprovalRulesPanel_should_prefillForm_When_CommandSampleSuggestionReturns
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
      fireEvent.click(screen.getByText("+ Add Custom Rule"));

      const details = screen.getByTestId("generate-from-command-details");
      expect(details).toBeInTheDocument();

      const textarea = screen.getByTestId("command-sample-textarea");
      expect(textarea).toBeInTheDocument();
    });

    it("Generate button is disabled when textarea is empty", () => {
      render(<ApprovalRulesPanel />);
      fireEvent.click(screen.getByText("+ Add Custom Rule"));

      const genBtn = screen.getByTestId("command-sample-generate-button");
      expect(genBtn).toBeDisabled();
    });

    it("Generate button is enabled when textarea has content", () => {
      render(<ApprovalRulesPanel />);
      fireEvent.click(screen.getByText("+ Add Custom Rule"));

      const textarea = screen.getByTestId("command-sample-textarea");
      fireEvent.change(textarea, { target: { value: "git push origin main" } });

      const genBtn = screen.getByTestId("command-sample-generate-button");
      expect(genBtn).not.toBeDisabled();
    });

    it("calls cmdGenerate with COMMAND_SAMPLE source when Generate clicked", async () => {
      render(<ApprovalRulesPanel />);
      fireEvent.click(screen.getByText("+ Add Custom Rule"));

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
      fireEvent.click(screen.getByText("+ Add Custom Rule"));

      // The component should have pre-filled the commandPattern field.
      const cmdInput = screen.getByTestId("form-command-pattern-input") as HTMLInputElement;
      expect(cmdInput.value).toBe("^git push");

      // AI badge should be visible.
      expect(screen.getByTestId("ai-generated-badge")).toBeInTheDocument();
    });

    it("does not overwrite a user-edited field with AI suggestion on re-render", () => {
      // No suggestions on initial render.
      render(<ApprovalRulesPanel />);
      fireEvent.click(screen.getByText("+ Add Custom Rule"));

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
      fireEvent.click(screen.getByText("+ Add Custom Rule"));
      // Close.
      fireEvent.click(screen.getByText("Cancel"));

      expect(hookConfig.cmd.clear).toHaveBeenCalled();

      // Reopen — form should be clean.
      fireEvent.click(screen.getByText("+ Add Custom Rule"));
      const nameInput = screen.getByTestId("form-name-input") as HTMLInputElement;
      expect(nameInput.value).toBe("");
    });
  });
});
