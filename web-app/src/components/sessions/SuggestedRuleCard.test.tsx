/**
 * Tests for SuggestedRuleCard component.
 *
 * Covers:
 *  - SuggestedRuleCard_should_showConfidenceBadge_When_Rendered
 *  - SuggestedRuleCard_should_showConflictWarning_When_ShadowedByRulesPresent
 *  - SuggestedRuleCard_should_callUpsertRule_When_AcceptClicked
 *  - SuggestedRuleCard_should_callOnDiscard_When_DiscardClicked
 *  - SuggestedRuleCard_should_allowEditingCommandPattern_Before_Accept
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { SuggestedRuleCard } from "./SuggestedRuleCard";
import type { SuggestedRuleProto, ApprovalRuleProto } from "@/gen/session/v1/types_pb";
import { AutoDecision } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockUpsertRule = jest.fn();
const mockRules: ApprovalRuleProto[] = [];

jest.mock("@/lib/hooks/useApprovalRules", () => ({
  useApprovalRules: () => ({
    rules: mockRules,
    loading: false,
    error: null,
    upsertRule: mockUpsertRule,
    deleteRule: jest.fn(),
    refresh: jest.fn(),
  }),
}));

// Vanilla-extract CSS modules are returned as empty strings in jsdom.
jest.mock("./SuggestedRuleCard.css", () => ({
  card: "card",
  cardHeader: "cardHeader",
  cardTitle: "cardTitle",
  confidenceBadge: () => "confidenceBadge",
  explanationBlock: "explanationBlock",
  sourceCommandsDetails: "sourceCommandsDetails",
  sourceCommandsSummary: "sourceCommandsSummary",
  sourceCommandsPre: "sourceCommandsPre",
  conflictBanner: "conflictBanner",
  shadowBanner: "shadowBanner",
  fieldGrid: "fieldGrid",
  fieldRow: "fieldRow",
  fieldLabel: "fieldLabel",
  fieldInput: "fieldInput",
  fieldSelect: "fieldSelect",
  fieldTextarea: "fieldTextarea",
  actions: "actions",
  acceptButton: "acceptButton",
  discardButton: "discardButton",
}));

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

function makeSuggestion(overrides: Partial<Record<string, unknown>> = {}): SuggestedRuleProto {
  return {
    name: "Allow git status",
    toolName: "Bash",
    toolPattern: "",
    commandPattern: "^git status",
    filePattern: "",
    decision: AutoDecision.ALLOW,
    riskLevel: "low",
    reason: "Safe read-only git command",
    alternative: "",
    priority: 100,
    confidence: 0.9,
    explanation: "Matches all git status invocations.",
    sourceCommands: ["git status", "git status --short"],
    shadowedByRuleIds: [],
    shadowsRuleIds: [],
    ...overrides,
  } as unknown as SuggestedRuleProto;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("SuggestedRuleCard", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockUpsertRule.mockResolvedValue(undefined);
  });

  describe("confidence badge", () => {
    it("SuggestedRuleCard_should_showConfidenceBadge_When_Rendered", () => {
      const suggestion = makeSuggestion({ confidence: 0.9 });

      render(
        <SuggestedRuleCard
          suggestion={suggestion}
          onAccept={jest.fn()}
          onDiscard={jest.fn()}
        />
      );

      const badge = screen.getByTestId("confidence-badge");
      expect(badge).toBeInTheDocument();
      expect(badge).toHaveTextContent("90% confidence");
      // High confidence (>=0.8) → level="high"
      expect(badge).toHaveAttribute("data-level", "high");
    });

    it("shows medium level badge when confidence is between 0.5 and 0.8", () => {
      render(
        <SuggestedRuleCard
          suggestion={makeSuggestion({ confidence: 0.65 })}
          onAccept={jest.fn()}
          onDiscard={jest.fn()}
        />
      );

      const badge = screen.getByTestId("confidence-badge");
      expect(badge).toHaveAttribute("data-level", "medium");
      expect(badge).toHaveTextContent("65% confidence");
    });

    it("shows low level badge when confidence is below 0.5", () => {
      render(
        <SuggestedRuleCard
          suggestion={makeSuggestion({ confidence: 0.3 })}
          onAccept={jest.fn()}
          onDiscard={jest.fn()}
        />
      );

      const badge = screen.getByTestId("confidence-badge");
      expect(badge).toHaveAttribute("data-level", "low");
    });
  });

  describe("conflict warnings", () => {
    it("SuggestedRuleCard_should_showConflictWarning_When_ShadowedByRulesPresent", () => {
      const suggestion = makeSuggestion({
        shadowedByRuleIds: ["rule-001", "rule-002"],
      });

      render(
        <SuggestedRuleCard
          suggestion={suggestion}
          onAccept={jest.fn()}
          onDiscard={jest.fn()}
        />
      );

      const banner = screen.getByTestId("conflict-banner");
      expect(banner).toBeInTheDocument();
      expect(banner).toHaveTextContent("May be shadowed by higher-priority rule(s):");
      expect(banner).toHaveTextContent("rule-001");
      expect(banner).toHaveTextContent("rule-002");
    });

    it("does not show conflict banner when shadowedByRuleIds is empty", () => {
      render(
        <SuggestedRuleCard
          suggestion={makeSuggestion({ shadowedByRuleIds: [] })}
          onAccept={jest.fn()}
          onDiscard={jest.fn()}
        />
      );

      expect(screen.queryByTestId("conflict-banner")).not.toBeInTheDocument();
    });

    it("shows shadow banner when shadowsRuleIds is non-empty", () => {
      render(
        <SuggestedRuleCard
          suggestion={makeSuggestion({ shadowsRuleIds: ["rule-999"] })}
          onAccept={jest.fn()}
          onDiscard={jest.fn()}
        />
      );

      const banner = screen.getByTestId("shadow-banner");
      expect(banner).toBeInTheDocument();
      expect(banner).toHaveTextContent("May suppress lower-priority rule(s):");
      expect(banner).toHaveTextContent("rule-999");
    });
  });

  describe("accept button", () => {
    it("SuggestedRuleCard_should_callUpsertRule_When_AcceptClicked", async () => {
      const onAccept = jest.fn();
      render(
        <SuggestedRuleCard
          suggestion={makeSuggestion()}
          onAccept={onAccept}
          onDiscard={jest.fn()}
        />
      );

      fireEvent.click(screen.getByTestId("accept-rule"));

      await waitFor(() => expect(mockUpsertRule).toHaveBeenCalledTimes(1));

      expect(mockUpsertRule).toHaveBeenCalledWith(
        expect.objectContaining({
          name: "Allow git status",
          toolName: "Bash",
          commandPattern: "^git status",
          decision: AutoDecision.ALLOW,
          enabled: true,
          source: "user",
        })
      );
    });

    it("accept button is disabled when name field is empty", async () => {
      render(
        <SuggestedRuleCard
          suggestion={makeSuggestion({ name: "" })}
          onAccept={jest.fn()}
          onDiscard={jest.fn()}
        />
      );

      const btn = screen.getByTestId("accept-rule");
      expect(btn).toBeDisabled();
    });

    it("accept button is disabled while saving", async () => {
      let resolveUpsert!: () => void;
      mockUpsertRule.mockReturnValue(
        new Promise<void>((resolve) => {
          resolveUpsert = resolve;
        })
      );

      render(
        <SuggestedRuleCard
          suggestion={makeSuggestion()}
          onAccept={jest.fn()}
          onDiscard={jest.fn()}
        />
      );

      fireEvent.click(screen.getByTestId("accept-rule"));

      await waitFor(() =>
        expect(screen.getByTestId("accept-rule")).toHaveTextContent("Saving…")
      );

      expect(screen.getByTestId("accept-rule")).toBeDisabled();

      // Settle.
      resolveUpsert();
      await waitFor(() =>
        expect(screen.getByTestId("accept-rule")).not.toHaveTextContent("Saving…")
      );
    });
  });

  describe("discard button", () => {
    it("SuggestedRuleCard_should_callOnDiscard_When_DiscardClicked", () => {
      const onDiscard = jest.fn();
      render(
        <SuggestedRuleCard
          suggestion={makeSuggestion()}
          onAccept={jest.fn()}
          onDiscard={onDiscard}
        />
      );

      fireEvent.click(screen.getByTestId("discard-rule"));

      expect(onDiscard).toHaveBeenCalledTimes(1);
      expect(mockUpsertRule).not.toHaveBeenCalled();
    });
  });

  describe("editable fields", () => {
    it("SuggestedRuleCard_should_allowEditingCommandPattern_Before_Accept", async () => {
      const onAccept = jest.fn();
      render(
        <SuggestedRuleCard
          suggestion={makeSuggestion({ commandPattern: "^git status" })}
          onAccept={onAccept}
          onDiscard={jest.fn()}
        />
      );

      const input = screen.getByTestId("field-command-pattern") as HTMLInputElement;
      expect(input.value).toBe("^git status");

      fireEvent.change(input, { target: { value: "^git (status|log)" } });
      expect(input.value).toBe("^git (status|log)");

      fireEvent.click(screen.getByTestId("accept-rule"));

      await waitFor(() => expect(mockUpsertRule).toHaveBeenCalledTimes(1));

      expect(mockUpsertRule).toHaveBeenCalledWith(
        expect.objectContaining({ commandPattern: "^git (status|log)" })
      );
    });

    it("pre-fills all editable fields from suggestion", () => {
      render(
        <SuggestedRuleCard
          suggestion={makeSuggestion({
            name: "Test Rule",
            toolName: "Bash",
            reason: "My reason",
            alternative: "Use X instead",
            priority: 200,
          })}
          onAccept={jest.fn()}
          onDiscard={jest.fn()}
        />
      );

      expect((screen.getByTestId("field-name") as HTMLInputElement).value).toBe("Test Rule");
      expect((screen.getByTestId("field-tool-name") as HTMLInputElement).value).toBe("Bash");
      expect((screen.getByTestId("field-reason") as HTMLTextAreaElement).value).toBe("My reason");
      expect((screen.getByTestId("field-alternative") as HTMLTextAreaElement).value).toBe("Use X instead");
      expect((screen.getByTestId("field-priority") as HTMLInputElement).value).toBe("200");
    });
  });

  describe("loading skeleton", () => {
    it("renders loading state when loading prop is true", () => {
      render(
        <SuggestedRuleCard
          suggestion={makeSuggestion()}
          onAccept={jest.fn()}
          onDiscard={jest.fn()}
          loading
        />
      );

      expect(screen.getByText("Generating suggestion…")).toBeInTheDocument();
      expect(screen.queryByTestId("confidence-badge")).not.toBeInTheDocument();
    });
  });
});
