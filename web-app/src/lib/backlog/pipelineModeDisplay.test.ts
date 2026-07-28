import { resolvePipelineModeDisplay } from "./pipelineModeDisplay";
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
