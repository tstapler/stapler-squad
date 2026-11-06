import { loadWindowLayout } from "../windowPersistence";
import { loadPaneLayout } from "@/lib/pane/usePaneLayout";
import v1Fixture from "./fixtures/v1-pane-layout.fixture.json";

const PANE_LAYOUT_KEY = "cockpit.paneLayout";
const WINDOW_LAYOUT_KEY = "cockpit.windowLayout";

beforeEach(() => {
  localStorage.clear();
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  jest.restoreAllMocks();
});

describe("loadWindowLayout migration from v1", () => {
  it("windowPersistence_should_migrateCapturedV1FixtureToWindow1ByteForByte_When_noV2KeyPresent", () => {
    const fixtureJson = JSON.stringify(v1Fixture);
    localStorage.setItem(PANE_LAYOUT_KEY, fixtureJson);

    const result = loadWindowLayout();

    expect(result).not.toBeNull();
    expect(result!.windows).toHaveLength(1);
    expect(result!.windows[0].name).toBe("Window 1");
    expect(result!.windows[0].paneState.root).toEqual(v1Fixture.root);
    expect(result!.windows[0].paneState.focusedPaneId).toBe(v1Fixture.focusedPaneId);
    expect(result!.windows[0].paneState.zoomedPaneId).toBe(v1Fixture.zoomedPaneId);

    // migration wrote through to the v2 key ...
    expect(localStorage.getItem(WINDOW_LAYOUT_KEY)).not.toBeNull();
    // ... but never touched the original v1 blob
    expect(localStorage.getItem(PANE_LAYOUT_KEY)).toBe(fixtureJson);
  });

  it("windowPersistence_should_failClosedToNull_When_v1RootIsMalformed", () => {
    localStorage.setItem(PANE_LAYOUT_KEY, JSON.stringify({ version: 1, root: null }));

    expect(() => loadWindowLayout()).not.toThrow();
    expect(loadWindowLayout()).toBeNull();
  });

  it("windowPersistence_should_returnNull_When_noKeysPresent", () => {
    expect(loadWindowLayout()).toBeNull();
  });

  it("windowPersistence_should_fallThroughToMigration_When_versionIsUnrecognized", () => {
    localStorage.setItem(WINDOW_LAYOUT_KEY, JSON.stringify({ version: 3, windows: [] }));
    // No v1 data either, so migration also yields null.
    const result = loadWindowLayout();
    expect(result).toBeNull();
    expect(console.error).toHaveBeenCalled();
  });
});

describe("migration_should_be_reversible", () => {
  it("migration_should_be_reversible", () => {
    const fixtureJson = JSON.stringify(v1Fixture);

    // 1. Up: seed v1 only, migrate, assert write-through + v1 untouched.
    localStorage.setItem(PANE_LAYOUT_KEY, fixtureJson);
    const migrated = loadWindowLayout();
    expect(migrated).not.toBeNull();
    expect(migrated!.version).toBe(2);
    expect(migrated!.revision).toBe(0);
    expect(migrated!.windows[0].name).toBe("Window 1");
    expect(localStorage.getItem(WINDOW_LAYOUT_KEY)).not.toBeNull();
    expect(localStorage.getItem(PANE_LAYOUT_KEY)).toBe(fixtureJson);

    // 2. Down: revert to the pre-migration code path directly. It must see the
    // original v1 layout exactly as it was, unaffected by the interim v2 write.
    const reverted = loadPaneLayout();
    expect(reverted).not.toBeNull();
    expect(reverted).toEqual(v1Fixture);

    // 3. Fail-closed, no data loss: a corrupt v1 blob never writes cockpit.windowLayout.
    localStorage.clear();
    localStorage.setItem(PANE_LAYOUT_KEY, JSON.stringify({ version: 1, root: null }));
    const failed = loadWindowLayout();
    expect(failed).toBeNull();
    expect(localStorage.getItem(WINDOW_LAYOUT_KEY)).toBeNull();
    expect(console.error).toHaveBeenCalled();
  });
});
