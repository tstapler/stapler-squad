import { loadPaneLayout, savePaneLayout, validateAndRepair, clearPaneLayout } from "../usePaneLayout";
import { PaneState, PaneNode, LeafPane, SplitPane } from "../paneTypes";

const LS_KEY = "cockpit.paneLayout";

beforeEach(() => {
  localStorage.clear();
});

// ─── loadPaneLayout ───────────────────────────────────────────────────────────

describe("loadPaneLayout", () => {
  it("usePaneLayout_should_returnNull_When_localStorageIsEmpty", () => {
    expect(loadPaneLayout()).toBeNull();
  });

  it("usePaneLayout_should_returnNull_When_storedJsonIsMalformed", () => {
    localStorage.setItem(LS_KEY, "not-valid-json{{");
    expect(loadPaneLayout()).toBeNull();
  });

  it("usePaneLayout_should_returnNull_When_versionIsNotOne", () => {
    localStorage.setItem(
      LS_KEY,
      JSON.stringify({ version: 2, root: {}, focusedPaneId: "x", zoomedPaneId: null })
    );
    expect(loadPaneLayout()).toBeNull();
  });

  it("usePaneLayout_should_returnNull_When_requiredFieldMissing", () => {
    // Missing focusedPaneId
    localStorage.setItem(
      LS_KEY,
      JSON.stringify({
        version: 1,
        root: { type: "leaf", id: "x", viewKind: "session-detail", sessionId: null, activeTab: "terminal" },
      })
    );
    expect(loadPaneLayout()).toBeNull();
  });

  it("usePaneLayout_should_returnParsedLayout_When_validJsonStored", () => {
    const layout = {
      version: 1,
      root: { type: "leaf", id: "abc", viewKind: "session-detail", sessionId: null, activeTab: "terminal" },
      focusedPaneId: "abc",
      zoomedPaneId: null,
    };
    localStorage.setItem(LS_KEY, JSON.stringify(layout));
    const result = loadPaneLayout();
    expect(result).not.toBeNull();
    expect(result?.focusedPaneId).toBe("abc");
  });
});

// ─── savePaneLayout ───────────────────────────────────────────────────────────

describe("savePaneLayout", () => {
  it("usePaneLayout_should_writeToLocalStorage_When_saveCalledWithValidState", () => {
    const state: PaneState = {
      root: { type: "leaf", id: "abc", viewKind: "session-detail", sessionId: null, activeTab: "terminal" },
      focusedPaneId: "abc",
      zoomedPaneId: null,
    };
    savePaneLayout(state);
    const stored = localStorage.getItem(LS_KEY);
    expect(stored).not.toBeNull();
    const parsed = JSON.parse(stored!);
    expect(parsed.version).toBe(1);
    expect(parsed.focusedPaneId).toBe("abc");
  });
});

// ─── clearPaneLayout ──────────────────────────────────────────────────────────

describe("clearPaneLayout", () => {
  it("usePaneLayout_should_removeFromLocalStorage_When_clearCalled", () => {
    localStorage.setItem(LS_KEY, "{}");
    clearPaneLayout();
    expect(localStorage.getItem(LS_KEY)).toBeNull();
  });
});

// ─── validateAndRepair ────────────────────────────────────────────────────────

describe("validateAndRepair", () => {
  it("usePaneLayout_should_nullifySessionId_When_sessionIdNotInValidSet", () => {
    const tree: PaneNode = {
      type: "leaf",
      id: "p1",
      viewKind: "session-detail",
      sessionId: "stale-session",
      activeTab: "terminal",
    };
    const validIds = new Set<string>(["other-session"]);
    const repaired = validateAndRepair(tree, validIds);
    expect((repaired as LeafPane).sessionId).toBeNull();
  });

  it("usePaneLayout_should_keepSessionId_When_sessionIdIsInValidSet", () => {
    const tree: PaneNode = {
      type: "leaf",
      id: "p1",
      viewKind: "session-detail",
      sessionId: "live-session",
      activeTab: "terminal",
    };
    const validIds = new Set<string>(["live-session"]);
    const repaired = validateAndRepair(tree, validIds);
    expect((repaired as LeafPane).sessionId).toBe("live-session");
  });

  it("usePaneLayout_should_nullifyOnlyStaleLeaves_When_treeHasMixedSessionIds", () => {
    const tree: PaneNode = {
      type: "split",
      id: "s1",
      direction: "vertical",
      ratio: 0.5,
      first: { type: "leaf", id: "p1", viewKind: "session-detail", sessionId: "live", activeTab: "terminal" },
      second: { type: "leaf", id: "p2", viewKind: "session-detail", sessionId: "stale", activeTab: "terminal" },
    };
    const validIds = new Set<string>(["live"]);
    const repaired = validateAndRepair(tree, validIds) as SplitPane;
    expect((repaired.first as LeafPane).sessionId).toBe("live");
    expect((repaired.second as LeafPane).sessionId).toBeNull();
  });

  it("usePaneLayout_should_keepNullSessionId_When_sessionIdIsAlreadyNull", () => {
    const tree: PaneNode = {
      type: "leaf",
      id: "p1",
      viewKind: "session-detail",
      sessionId: null,
      activeTab: "terminal",
    };
    const validIds = new Set<string>(["session-A"]);
    const repaired = validateAndRepair(tree, validIds);
    expect((repaired as LeafPane).sessionId).toBeNull();
  });
});
