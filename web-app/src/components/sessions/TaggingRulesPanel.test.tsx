/**
 * Tests Story 6.1.1's tab-content behavior: real fire-count data (not a
 * placeholder), the built-in fire-count tooltip reused verbatim from
 * ApprovalRulesPanel, and the per-rule enable/disable toggle (Task 6.1.1e).
 */

import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { TaggingRuleProtoSchema } from "@/gen/session/v1/types_pb";
import type { TaggingRuleProto } from "@/gen/session/v1/types_pb";
import { TaggingRulesPanel } from "./TaggingRulesPanel";

const upsertRule = jest.fn().mockResolvedValue(undefined);
const deleteRule = jest.fn().mockResolvedValue(undefined);
let mockRules: TaggingRuleProto[] = [];
let mockLoading = false;

jest.mock("@/lib/hooks/useTaggingRules", () => ({
  useTaggingRules: () => ({
    rules: mockRules,
    loading: mockLoading,
    error: null,
    upsertRule,
    deleteRule,
    refresh: jest.fn(),
  }),
}));

function buildRule(overrides: Partial<Omit<TaggingRuleProto, "$typeName" | "$unknown">> = {}): TaggingRuleProto {
  return create(TaggingRuleProtoSchema, {
    id: "seed-bugfix",
    name: "Bugfix branch",
    branchPattern: "^(bugfix|fix)/",
    outputTag: "Bugfix",
    priority: 50,
    enabled: true,
    source: "user",
    fireCount7d: 12,
    ...overrides,
  });
}

describe("TaggingRulesPanel", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockLoading = false;
    mockRules = [];
  });

  it("TaggingRulesPanel_should_ShowRealNonZeroFireCount_When_RuleHasFiredInLast7Days", () => {
    mockRules = [buildRule({ fireCount7d: 12 })];
    render(<TaggingRulesPanel />);

    expect(screen.getByText("Bugfix branch")).not.toBeNull();
    const fireCountCell = screen.getByText("12");
    expect(fireCountCell).not.toBeNull();
  });

  it("TaggingRulesPanel_should_ReuseApprovalRuleFireCountTooltipVerbatim_When_ColumnHeaderRenders", () => {
    mockRules = [buildRule()];
    render(<TaggingRulesPanel />);

    const header = screen.getByText("Fires (7d)");
    expect(header.getAttribute("title")).toBe("Number of times this rule fired in the last 7 days");
  });

  it("TaggingRulesPanel_should_ShowPassiveDash_When_RuleHasZeroFires", () => {
    mockRules = [buildRule({ fireCount7d: 0, name: "Claude program" })];
    render(<TaggingRulesPanel />);

    expect(screen.getByText("—")).not.toBeNull();
  });

  it("TaggingRulesPanel_should_ShowEmptyState_When_NoRulesConfigured", () => {
    mockRules = [];
    render(<TaggingRulesPanel />);

    expect(screen.getByTestId("tagging-rules-empty-state")).not.toBeNull();
  });

  it("TaggingRulesPanel_should_UpsertWithFlippedEnabled_When_ToggleClicked", async () => {
    mockRules = [buildRule({ enabled: true })];
    render(<TaggingRulesPanel />);

    fireEvent.click(screen.getByRole("button", { name: "Disable tagging rule" }));

    await waitFor(() => expect(upsertRule).toHaveBeenCalledTimes(1));
    expect(upsertRule.mock.calls[0][0]).toMatchObject({ id: "seed-bugfix", enabled: false });
  });

  it("TaggingRulesPanel_should_ShowAlwaysOnBadge_When_RuleIsSeedSourced", () => {
    mockRules = [buildRule({ source: "seed", enabled: true })];
    render(<TaggingRulesPanel />);

    expect(screen.getByText("Always on")).not.toBeNull();
    expect(screen.queryByRole("button", { name: /Disable tagging rule/ })).toBeNull();
  });

  it("TaggingRulesPanel_should_ShowBuiltInBadgeAndHideEditDelete_When_RuleIsSeedSourced", () => {
    mockRules = [buildRule({ source: "seed", name: "Feature branch" })];
    render(<TaggingRulesPanel />);

    expect(screen.getByTitle("Built-in")).not.toBeNull();
    expect(screen.queryByRole("button", { name: /Edit tagging rule/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /Delete tagging rule/ })).toBeNull();
  });

  it("TaggingRulesPanel_should_NotShowBuiltInBadge_When_RuleIsUserSourced", () => {
    mockRules = [buildRule({ source: "user", name: "My rule" })];
    render(<TaggingRulesPanel />);

    expect(screen.queryByTitle("Built-in")).toBeNull();
    expect(screen.getByRole("button", { name: /Edit tagging rule/ })).not.toBeNull();
  });
});
