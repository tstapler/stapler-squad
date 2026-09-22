import { resolvePipelineModeDisplay, resolveExecutorProvenance } from "./pipelineModeDisplay";
import type { LinkedSession, PipelineMode } from "@/lib/hooks/useBacklogService";

function makeSession(
  overrides: Partial<Pick<LinkedSession, "pipelineModeSnapshot" | "pipelineModeSnapshotHash">>
): Pick<LinkedSession, "pipelineModeSnapshot" | "pipelineModeSnapshotHash"> {
  return {
    pipelineModeSnapshot: "",
    pipelineModeSnapshotHash: "",
    ...overrides,
  };
}

function makeMode(overrides: Partial<PipelineMode>): PipelineMode {
  return {
    id: "mode-1",
    slug: "custom-mode",
    name: "Custom Mode",
    description: "",
    enabled: true,
    statusCommandTemplate: "",
    doneCommandTemplate: "",
    failCommandTemplate: "",
    reviewCommandTemplate: "",
    shipCommandTemplate: "",
    helpCommandTemplate: "",
    triagePromptTemplate: "",
    reviewPromptTemplate: "",
    initialPromptTemplate: "",
    contentHash: "hash-a",
    ...overrides,
  };
}

describe("resolvePipelineModeDisplay", () => {
  it("resolvePipelineModeDisplay_should_ReturnDefaultResolved_When_SnapshotIsEmpty", () => {
    const session = makeSession({ pipelineModeSnapshot: "" });

    expect(resolvePipelineModeDisplay(session, [])).toEqual({
      kind: "resolved",
      name: "default",
      drifted: false,
    });
  });

  it("resolvePipelineModeDisplay_should_ReturnDefaultResolved_When_SnapshotIsUndefined", () => {
    const session: Pick<LinkedSession, "pipelineModeSnapshot" | "pipelineModeSnapshotHash"> = {};

    expect(resolvePipelineModeDisplay(session, [makeMode({ slug: "custom-mode" })])).toEqual({
      kind: "resolved",
      name: "default",
      drifted: false,
    });
  });

  it("resolvePipelineModeDisplay_should_ReturnUnrecognized_When_SnapshotSlugNotInModeList", () => {
    const session = makeSession({ pipelineModeSnapshot: "deleted-mode", pipelineModeSnapshotHash: "hash-a" });
    const modes = [makeMode({ slug: "custom-mode" })];

    expect(resolvePipelineModeDisplay(session, modes)).toEqual({
      kind: "unrecognized",
      slug: "deleted-mode",
    });
  });

  it("resolvePipelineModeDisplay_should_ReturnDriftedResolved_When_SlugFoundButContentHashChanged", () => {
    const session = makeSession({ pipelineModeSnapshot: "custom-mode", pipelineModeSnapshotHash: "hash-old" });
    const modes = [makeMode({ slug: "custom-mode", name: "Custom Mode", contentHash: "hash-new" })];

    expect(resolvePipelineModeDisplay(session, modes)).toEqual({
      kind: "resolved",
      name: "Custom Mode",
      drifted: true,
    });
  });

  it("resolvePipelineModeDisplay_should_ReturnNotDriftedResolved_When_SlugFoundAndHashMatches", () => {
    const session = makeSession({ pipelineModeSnapshot: "custom-mode", pipelineModeSnapshotHash: "hash-a" });
    const modes = [makeMode({ slug: "custom-mode", name: "Custom Mode", contentHash: "hash-a" })];

    expect(resolvePipelineModeDisplay(session, modes)).toEqual({
      kind: "resolved",
      name: "Custom Mode",
      drifted: false,
    });
  });

  it("resolvePipelineModeDisplay_should_ReturnNotDriftedResolved_When_SlugFoundAndSnapshotHashIsEmpty", () => {
    // A pre-feature session: the snapshot slug exists (mode was created after
    // the session ran, or the hash simply wasn't captured), but there's no
    // recorded hash to compare against, so drift can't be detected — treated
    // as not drifted rather than assumed drifted.
    const session = makeSession({ pipelineModeSnapshot: "custom-mode", pipelineModeSnapshotHash: "" });
    const modes = [makeMode({ slug: "custom-mode", name: "Custom Mode", contentHash: "hash-a" })];

    expect(resolvePipelineModeDisplay(session, modes)).toEqual({
      kind: "resolved",
      name: "Custom Mode",
      drifted: false,
    });
  });
});

// ─── Story 5.2.4: resolveExecutorProvenance ────────────────────────────────

type ExecutorSession = Pick<
  LinkedSession,
  "role" | "configuredProgram" | "resolvedProgram" | "resolvedModel" | "executorFallbackReason" | "executorSnapshotHash"
>;

function makeExecutorSession(overrides: Partial<ExecutorSession> = {}): ExecutorSession {
  return {
    role: "triage",
    configuredProgram: "",
    resolvedProgram: "",
    resolvedModel: "",
    executorFallbackReason: "",
    executorSnapshotHash: "",
    ...overrides,
  };
}

function makeExecutorMode(hashes: Record<string, string>): Pick<PipelineMode, "stageExecutorHashes"> {
  return { stageExecutorHashes: hashes };
}

describe("resolveExecutorProvenance", () => {
  it("resolveExecutorProvenance_should_ReturnNullFallbackAndNotDrifted_When_NeitherFactPresent", () => {
    const session = makeExecutorSession({ executorSnapshotHash: "hash-a" });
    const mode = makeExecutorMode({ triage: "hash-a" });

    expect(resolveExecutorProvenance(session, mode)).toEqual({ fallback: null, drifted: false });
  });

  it("resolveExecutorProvenance_should_ReturnFallbackInfo_When_ExecutorFallbackReasonNonEmpty", () => {
    const session = makeExecutorSession({
      configuredProgram: "gemini",
      resolvedProgram: "",
      resolvedModel: "claude-haiku-4-5",
      executorFallbackReason: "gemini_unavailable",
    });

    expect(resolveExecutorProvenance(session, undefined)).toEqual({
      fallback: {
        configuredProgram: "gemini",
        resolvedProgram: "",
        resolvedModel: "claude-haiku-4-5",
        reason: "gemini_unavailable",
      },
      drifted: false,
    });
  });

  it("resolveExecutorProvenance_should_ReturnDriftedTrue_When_SnapshotHashMismatchesModeHashForRole", () => {
    const session = makeExecutorSession({ role: "triage", executorSnapshotHash: "a1b2c3d4e5f6a1b2" });
    const mode = makeExecutorMode({ triage: "f6e5d4c3b2a1f6e5" });

    expect(resolveExecutorProvenance(session, mode)).toEqual({ fallback: null, drifted: true });
  });

  it("resolveExecutorProvenance_should_ReturnNotDrifted_When_UnconfiguredRoleHashesMatchByConstruction", () => {
    // Regression test for the dense-hash fix (Task 5.2.4a): a role with no
    // configured override still gets a real hash on both the session side
    // (ComputeExecutorHash("", "")) and the mode side (Task 5.2.4a's dense
    // map) — they must match, not read as drifted just because the role was
    // never explicitly configured.
    const defaultHash = "0000default0000a"; // stand-in for ComputeExecutorHash("", "")
    const session = makeExecutorSession({ role: "review", executorSnapshotHash: defaultHash });
    const mode = makeExecutorMode({ review: defaultHash });

    expect(resolveExecutorProvenance(session, mode)).toEqual({ fallback: null, drifted: false });
  });

  it("resolveExecutorProvenance_should_ReturnNotDrifted_When_SessionPredatesFeatureAndSnapshotHashIsEmpty", () => {
    const session = makeExecutorSession({ role: "work", executorSnapshotHash: "" });
    const mode = makeExecutorMode({ work: "some-hash" });

    expect(resolveExecutorProvenance(session, mode)).toEqual({ fallback: null, drifted: false });
  });

  it("resolveExecutorProvenance_should_ReturnBothFallbackAndDrifted_When_BothFactsPresentSimultaneously", () => {
    // Regression test for the case-priority fix (Task 5.2.4b): fallback and
    // drift are independent facts and must both be reported, never one
    // suppressing the other via if/else-if priority.
    const session = makeExecutorSession({
      role: "triage",
      configuredProgram: "gemini",
      executorFallbackReason: "gemini_unavailable",
      executorSnapshotHash: "hash-old",
    });
    const mode = makeExecutorMode({ triage: "hash-new" });

    const result = resolveExecutorProvenance(session, mode);
    expect(result.fallback).not.toBeNull();
    expect(result.drifted).toBe(true);
  });

  it("resolveExecutorProvenance_should_ReturnNotDrifted_When_FamilyAliasHashUnedited", () => {
    // Regression test for the family-alias hash fix: both the session-side
    // hash (Task 2.3.3b, hashed from the raw "family:opus" string) and the
    // mode-side hash (Task 5.2.4a, computed from that same raw string) are
    // opaque strings to the frontend — this test only proves the comparison
    // itself is a plain string match, unaffected by what the alias later
    // resolves to (resolvedModel is a concrete ID, irrelevant to drift).
    const rawAliasHash = "family-opus-hash";
    const session = makeExecutorSession({
      role: "review",
      executorSnapshotHash: rawAliasHash,
      resolvedModel: "claude-opus-4-8",
    });
    const mode = makeExecutorMode({ review: rawAliasHash });

    expect(resolveExecutorProvenance(session, mode)).toEqual({ fallback: null, drifted: false });
  });
});
