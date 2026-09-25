import { classifySegment, truncateWorkspacePath } from "./truncateWorkspacePath";
import { truncateMiddle } from "./truncateMiddle";

// Confirmed real-world fixture (this session's own worktree path) — see
// research/features.md's "Confirmed real-world opaque segment example".
const CANONICAL_WORKSPACE_PATH =
  "/Users/tstapler/.stapler-squad/workspaces/6eb0b580fa0331d5/worktrees/stapler-squad-wasted-space_18d807dfb97a2b28";
const WORKSPACE_HASH = "6eb0b580fa0331d5";
const WORKTREE_UUID_SUFFIX = "18d807dfb97a2b28";

describe("truncateWorkspacePath", () => {
  it("truncateWorkspacePath_should_ReturnUnchanged_When_PathUnderMaxLen", () => {
    expect(truncateWorkspacePath("~/repo/src", 40)).toBe("~/repo/src");
  });

  it("truncateWorkspacePath_should_CollapseHomeDirToTilde_When_AbsoluteUserPathGiven", () => {
    const result = truncateWorkspacePath("/Users/tstapler/repo/src", 40);
    expect(result.startsWith("~/repo/src")).toBe(true);
    expect(result).not.toContain("Users");
    expect(result).not.toContain("tstapler");
  });

  it("truncateWorkspacePath_should_CollapseHomeDirAndOpaqueSegment_When_BothApply", () => {
    const result = truncateWorkspacePath(CANONICAL_WORKSPACE_PATH, 72);
    expect(result.startsWith("~/")).toBe(true);
    expect(result).not.toContain(WORKSPACE_HASH);
  });

  it("truncateWorkspacePath_should_CollapseWorkspaceHashSegment_When_PathContainsOpaqueHash", () => {
    const path =
      "/Users/tstapler/.stapler-squad/workspaces/6eb0b580fa0331d5/worktrees/stapler-squad-wasted-space";
    const result = truncateWorkspacePath(path, 50);
    expect(result.startsWith("~/")).toBe(true);
    expect(result).toContain("…");
    expect(result).not.toContain(WORKSPACE_HASH);
  });

  it("truncateWorkspacePath_should_CollapseTrailingHexSuffix_When_SemanticSegmentHasOpaqueSuffix", () => {
    const path = "worktrees/stapler-squad-wasted-space_18d807dfb97a2b28";
    const result = truncateWorkspacePath(path, 40);
    expect(result).toContain("stapler-squad-wasted-space");
    expect(result).toContain("…");
    expect(result).not.toContain(WORKTREE_UUID_SUFFIX);
  });

  it("truncateWorkspacePath_should_CollapseTrailingHexRun_When_TokenHasNoPathSeparator", () => {
    const path = "pr-424-compute-nop-18c993e1e9402c8f1a";
    const result = truncateWorkspacePath(path, 25);
    expect(result.startsWith("pr-424-compute-nop")).toBe(true);
    expect(result.endsWith("…")).toBe(true);
    expect(result).not.toContain("18c993e1e9402c8f1a");
  });

  it("truncateWorkspacePath_should_PreserveShortHexLikeSegment_When_BelowLengthThreshold", () => {
    const path = "repo/deadbeef/src";
    expect(truncateWorkspacePath(path, 100)).toBe(path);
  });

  it("classifySegment_should_ReturnSemantic_When_HexLikeSegmentBelowSevenChars", () => {
    expect(classifySegment("dead")).toBe("semantic");
  });

  it("truncateWorkspacePath_should_FallBackToTruncateMiddle_When_NoOpaqueSegmentFound", () => {
    const path =
      "a-very-long-but-entirely-human-readable-branch-name-with-no-hash";
    expect(truncateWorkspacePath(path, 30)).toBe(truncateMiddle(path, 30));
  });

  it("truncateWorkspacePath_should_ReturnEmptyString_When_InputIsEmpty", () => {
    expect(truncateWorkspacePath("", 30)).toBe("");
  });

  // Regression pin (adversarial-review.md, 2026-09-24 re-review): after
  // home-dir + opaque-segment collapse the canonical fixture is exactly 67
  // chars, which fits under maxLen=72 without ever hitting the
  // truncateMiddle fallback — pins this so a future budget decrease can't
  // silently regress the trailing segment back into truncateMiddle's
  // embedded-dot fallback (which mid-word-truncates it).
  it("truncateWorkspacePath_should_KeepTrailingSegmentIntact_When_BudgetIs72", () => {
    const result = truncateWorkspacePath(CANONICAL_WORKSPACE_PATH, 72);

    // Pins the post-collapse, pre-fallback shape (67 chars, <= 72) so a
    // future budget decrease can't silently push this input back into
    // truncateMiddle's embedded-dot fallback, which would mid-word-truncate
    // "stapler-squad-wasted-space" instead of keeping it intact.
    expect(result).toBe(
      "~/.stapler-squad/workspaces/…/worktrees/stapler-squad-wasted-space…",
    );
    expect(result).toContain("stapler-squad-wasted-space");
    expect(result.length).toBeLessThanOrEqual(72);

    // The rejected `56` budget forces the embedded-dot fallback and
    // mid-word-truncates the trailing segment — confirms this test would
    // have caught that regression.
    const mangledAt56 = truncateWorkspacePath(CANONICAL_WORKSPACE_PATH, 56);
    expect(mangledAt56).not.toContain("stapler-squad-wasted-space");
  });
});
