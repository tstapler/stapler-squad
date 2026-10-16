/**
 * Story 6.1.1: the "Tagging Rules" tab lives alongside the existing
 * approval-rule source-filter tabs and, when selected, swaps the approval
 * table/builder out for TaggingRulesPanel's content.
 */

import { render, screen, fireEvent } from "@testing-library/react";
import { ApprovalRulesPanel } from "./ApprovalRulesPanel";

jest.mock("@/lib/hooks/useApprovalRules", () => ({
  useApprovalRules: () => ({
    rules: [],
    loading: false,
    error: null,
    upsertRule: jest.fn(),
    deleteRule: jest.fn(),
    refresh: jest.fn(),
    reloadClaudeSettingsRules: jest.fn(),
  }),
}));

jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({ showActionToast: jest.fn() }),
}));

jest.mock("@/lib/hooks/useApprovalAnalytics", () => ({
  useApprovalAnalytics: () => ({ summary: null, loading: false, error: null }),
}));

jest.mock("@/lib/hooks/useExportRules", () => ({
  useExportRules: () => ({ exportRules: jest.fn(), loading: false, error: null }),
}));

jest.mock("@/lib/hooks/useGenerateRule", () => ({
  useGenerateRule: () => ({
    suggestions: [], loading: false, error: null,
    generate: jest.fn(), cancel: jest.fn(), clear: jest.fn(),
  }),
}));

jest.mock("./ImportRulesModal", () => ({ ImportRulesModal: () => null }));

const mockTaggingRules = jest.fn();
jest.mock("@/lib/hooks/useTaggingRules", () => ({
  useTaggingRules: () => mockTaggingRules(),
}));

describe("ApprovalRulesPanel — Tagging Rules tab", () => {
  beforeEach(() => {
    mockTaggingRules.mockReturnValue({
      rules: [],
      loading: false,
      error: null,
      upsertRule: jest.fn(),
      deleteRule: jest.fn(),
      refresh: jest.fn(),
    });
  });

  it("ApprovalRulesPanel_should_ShowTaggingRulesTab_When_Rendered", () => {
    render(<ApprovalRulesPanel />);
    expect(screen.getByTestId("tagging-rules-tab")).not.toBeNull();
  });

  it("ApprovalRulesPanel_should_HideApprovalTable_When_TaggingRulesTabActive", () => {
    render(<ApprovalRulesPanel />);
    expect(screen.queryByTestId("tagging-rules-panel")).toBeNull();

    fireEvent.click(screen.getByTestId("tagging-rules-tab"));

    expect(screen.getByTestId("tagging-rules-panel")).not.toBeNull();
    expect(screen.queryByTestId("empty-state")).toBeNull();
  });

  it("ApprovalRulesPanel_should_ReturnToApprovalTable_When_SourceTabClickedAfterTaggingRulesTab", () => {
    render(<ApprovalRulesPanel />);
    fireEvent.click(screen.getByTestId("tagging-rules-tab"));
    expect(screen.getByTestId("tagging-rules-panel")).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /All \(0\)/ }));

    expect(screen.queryByTestId("tagging-rules-panel")).toBeNull();
  });
});
