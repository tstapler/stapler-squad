import { resolveJulesDispatchGate, type JulesConfigForGate } from "./julesDispatchGate";
import type { LinkedSession } from "@/lib/hooks/useBacklogService";

function makeConfig(overrides: Partial<JulesConfigForGate> = {}): JulesConfigForGate {
  return { enabled: true, hasApiKey: true, ...overrides };
}

function makeSession(overrides: Partial<LinkedSession> = {}): LinkedSession {
  return {
    entityId: "entity-1",
    sessionId: "session-1",
    role: "work",
    estimatedCostUsd: 0,
    ...overrides,
  };
}

describe("resolveJulesDispatchGate", () => {
  it("resolveJulesDispatchGate_should_ReturnHidden_When_JulesConfigIsNull", () => {
    expect(resolveJulesDispatchGate(null, [])).toEqual({ hidden: true, disabled: true, reason: null, branch: "" });
  });

  it("resolveJulesDispatchGate_should_ReturnHidden_When_EnabledFalse", () => {
    expect(resolveJulesDispatchGate(makeConfig({ enabled: false }), [])).toEqual({
      hidden: true,
      disabled: true,
      reason: null,
      branch: "",
    });
  });

  it("resolveJulesDispatchGate_should_DisableWithAddKeyReason_When_HasApiKeyFalse", () => {
    const gate = resolveJulesDispatchGate(makeConfig({ hasApiKey: false }), []);
    expect(gate).toEqual({
      hidden: false,
      disabled: true,
      reason: "Add a Jules API key in Settings to enable cloud sessions.",
      branch: "",
    });
  });

  it("resolveJulesDispatchGate_should_DisableWithAlreadyRunningReason_When_OpenJulesSessionPresent", () => {
    const sessions = [makeSession({ role: "jules_work", endedAt: undefined, worktreeBranch: "backlog/item-1" })];
    const gate = resolveJulesDispatchGate(makeConfig(), sessions);
    expect(gate).toEqual({
      hidden: false,
      disabled: true,
      reason: "A Jules session is already running for this item.",
      branch: "backlog/item-1",
    });
  });

  it("resolveJulesDispatchGate_should_NotCountEndedJulesSession_When_CheckingOpenSession", () => {
    const sessions = [
      makeSession({ role: "jules_work", endedAt: "2026-08-01T00:00:00Z", worktreeBranch: "backlog/item-1" }),
    ];
    const gate = resolveJulesDispatchGate(makeConfig(), sessions);
    expect(gate?.disabled).toBe(false);
  });

  it("resolveJulesDispatchGate_should_DisableWithNoBranchReason_When_ZeroSessionsCarryWorktreeBranch", () => {
    const sessions = [makeSession({ role: "work", worktreeBranch: undefined })];
    const gate = resolveJulesDispatchGate(makeConfig(), sessions);
    expect(gate).toEqual({
      hidden: false,
      disabled: true,
      reason: "This item has no branch yet — spawn a local session (or push a branch) before dispatching to Jules.",
      branch: "",
    });
  });

  it("resolveJulesDispatchGate_should_Enable_When_ConfiguredWithOpenBranchAndNoOpenSession", () => {
    const sessions = [makeSession({ role: "work", worktreeBranch: "backlog/item-1" })];
    const gate = resolveJulesDispatchGate(makeConfig(), sessions);
    expect(gate).toEqual({ hidden: false, disabled: false, reason: null, branch: "backlog/item-1" });
  });

  it("resolveJulesDispatchGate_should_PickNewestNonEmptyBranch_When_MultipleSessionsCarryOne", () => {
    const sessions = [
      makeSession({ entityId: "e1", worktreeBranch: "backlog/older" }),
      makeSession({ entityId: "e2", worktreeBranch: undefined }),
      makeSession({ entityId: "e3", worktreeBranch: "backlog/newest" }),
    ];
    const gate = resolveJulesDispatchGate(makeConfig(), sessions);
    expect(gate?.branch).toBe("backlog/newest");
  });

  it("resolveJulesDispatchGate_should_ShowOnlyNoKeyReason_When_BothNoKeyAndNoBranchApply", () => {
    // Precedence: the key check runs before the branch check, so a config
    // with neither a key nor a known branch must surface only the key
    // reason — never both, and never the branch reason instead.
    const gate = resolveJulesDispatchGate(makeConfig({ hasApiKey: false }), []);
    expect(gate?.reason).toBe("Add a Jules API key in Settings to enable cloud sessions.");
  });

  it("resolveJulesDispatchGate_should_ShowOnlyOpenSessionReason_When_BothOpenSessionAndNoBranchApply", () => {
    // Precedence: an open Jules session wins over the no-branch check.
    // Neither session here carries a worktreeBranch, so "no branch" would
    // also match if the open-session check ran second instead of first.
    const sessions = [makeSession({ role: "jules_work", worktreeBranch: undefined })];
    const gate = resolveJulesDispatchGate(makeConfig(), sessions);
    expect(gate?.reason).toBe("A Jules session is already running for this item.");
  });
});
