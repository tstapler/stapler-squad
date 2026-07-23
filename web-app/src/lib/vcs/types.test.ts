import type { VcsWidgetData } from "./types";

describe("VcsWidgetData", () => {
  it("VcsWidgetData_should_CompileLiveVariantWithNoSnapshotAt_When_KindIsLive", () => {
    const data: VcsWidgetData = {
      kind: "live",
      branch: "feat/foo",
      isClean: false,
      fileChanges: [],
      aheadOfMain: 2,
      behindMain: 0,
      branchExists: true,
      commits: [],
      github: null,
      shipped: false,
    };

    expect(data.kind).toBe("live");
    expect("snapshotAt" in data).toBe(false);
  });

  it("VcsWidgetData_should_RejectSnapshotAtOnLiveVariant_When_TypeChecked", () => {
    // The "live" branch has no snapshotAt field, so this combination must be
    // a structurally unrepresentable compile error, not just a convention.
    const data: VcsWidgetData = {
      kind: "live",
      branch: "feat/foo",
      isClean: false,
      fileChanges: [],
      aheadOfMain: 2,
      behindMain: 0,
      branchExists: true,
      commits: [],
      github: null,
      shipped: false,
      // @ts-expect-error — snapshotAt does not exist on the "live" variant
      snapshotAt: new Date(),
    };

    expect(data).toBeDefined();
  });

  it("VcsWidgetData_should_CompileHistoricalVariantWithSnapshotFields_When_KindIsHistorical", () => {
    const data: VcsWidgetData = {
      kind: "historical",
      branch: "feat/foo",
      isClean: true,
      fileChanges: [],
      aheadOfMain: 0,
      behindMain: 0,
      branchExists: false,
      commits: [],
      github: null,
      shipped: false,
      snapshotAt: null,
      snapshotCaptureFailed: true,
    };

    expect(data.kind).toBe("historical");
  });
});
