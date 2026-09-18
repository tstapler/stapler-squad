/**
 * Unit tests for SessionCard's `isRedundantWithTitle` and `basenameOf` predicates,
 * which drive title/subtitle deduplication (don't repeat title text in secondary rows).
 *
 * Tests the exported pure functions directly, mirroring
 * SessionCard.pending-program.test.tsx's no-render style — no component mounting needed.
 */

import { isRedundantWithTitle, basenameOf } from "../SessionCard";

describe("isRedundantWithTitle", () => {
  it("isRedundantWithTitle_should_ReturnTrue_When_ValueExactlyMatchesTitle", () => {
    expect(isRedundantWithTitle("fix-auth", "fix-auth")).toBe(true);
  });

  it("isRedundantWithTitle_should_ReturnFalse_When_ValueDiffersOnlyByCase", () => {
    expect(isRedundantWithTitle("Fix-Auth", "fix-auth")).toBe(false);
  });

  it("isRedundantWithTitle_should_ReturnTrue_When_ValueHasSurroundingWhitespace", () => {
    expect(isRedundantWithTitle(" fix-auth ", "fix-auth")).toBe(true);
  });

  it("isRedundantWithTitle_should_ReturnFalse_When_BasenameIsANearMissSubstring", () => {
    expect(isRedundantWithTitle(basenameOf("/home/user/worktrees/fix-auth-2"), "fix-auth")).toBe(false);
  });

  it("isRedundantWithTitle_should_ReturnFalse_When_ValueIsEmptyString", () => {
    expect(isRedundantWithTitle("", "fix-auth")).toBe(false);
  });

  it("isRedundantWithTitle_should_ReturnFalse_When_ValueIsUndefinedOrNull", () => {
    expect(isRedundantWithTitle(undefined, "fix-auth")).toBe(false);
    expect(isRedundantWithTitle(null, "fix-auth")).toBe(false);
  });
});

describe("basenameOf", () => {
  it("basenameOf_should_ReturnLastPathSegment_When_GivenAPlainAbsolutePath", () => {
    expect(basenameOf("/home/user/worktrees/fix-auth")).toBe("fix-auth");
  });

  it("basenameOf_should_ReturnWholeTrimmedPath_When_PathHasATrailingSlash", () => {
    // Documented inherited quirk from the existing repo idiom (split("/").pop() on a
    // trailing-slash path returns "", falls through to `|| trimmed`), not a new bug.
    expect(basenameOf("/home/user/worktrees/fix-auth/")).toBe("/home/user/worktrees/fix-auth/");
  });

  it("basenameOf_should_ReturnWholeString_When_PathHasNoSlash", () => {
    expect(basenameOf("fix-auth")).toBe("fix-auth");
  });

  it("basenameOf_should_TrimWhitespace_When_PathHasSurroundingWhitespace", () => {
    expect(basenameOf("  /tmp/clones/shared-fixes  ")).toBe("shared-fixes");
  });
});
