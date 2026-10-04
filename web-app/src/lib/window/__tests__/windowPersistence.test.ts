import { loadWindowLayout, saveWindowLayout } from "../windowPersistence";
import type { NamedWindow, PersistedWindowLayoutV2 } from "../windowTypes";

const WINDOW_LAYOUT_KEY = "cockpit.windowLayout";

beforeEach(() => {
  localStorage.clear();
});

describe("loadWindowLayout", () => {
  it("windowPersistence_should_returnParsedLayoutUnmodified_When_validV2KeyPresent", () => {
    const layout: PersistedWindowLayoutV2 = {
      version: 2,
      revision: 3,
      windows: [
        {
          id: "win-1",
          name: "Window 1",
          paneState: {
            root: { type: "leaf", id: "p1", viewKind: "session-detail", sessionId: null, activeTab: "terminal" },
            focusedPaneId: "p1",
            zoomedPaneId: null,
          },
        },
      ],
    };
    localStorage.setItem(WINDOW_LAYOUT_KEY, JSON.stringify(layout));

    const result = loadWindowLayout();

    expect(result).toEqual(layout);
  });

  it("windowPersistence_should_returnNull_When_neitherV2NorV1KeyPresent", () => {
    expect(loadWindowLayout()).toBeNull();
  });
});

function buildTwoWindowFixture(): NamedWindow[] {
  return [
    {
      id: "win-1",
      name: "Window 1",
      paneState: {
        root: { type: "leaf", id: "p1", viewKind: "session-detail", sessionId: "s1", activeTab: "diff" },
        focusedPaneId: "p1",
        zoomedPaneId: null,
      },
    },
    {
      id: "win-2",
      name: "Window 2",
      paneState: {
        root: {
          type: "split",
          id: "split-1",
          direction: "vertical",
          ratio: 0.4,
          first: { type: "leaf", id: "p2", viewKind: "session-list", sessionId: null, activeTab: "terminal" },
          second: { type: "leaf", id: "p3", viewKind: "session-detail", sessionId: "s3", activeTab: "logs" },
        },
        focusedPaneId: "p3",
        zoomedPaneId: "p3",
      },
    },
  ];
}

describe("save then load round trip", () => {
  it("windowPersistence_should_roundTripAllWindowFieldsThroughRealLocalStorage_When_saveThenLoad", () => {
    // Uses jsdom's real, unmocked localStorage (per validation.md) so
    // serialization bugs aren't hidden behind a mocked Storage stub.
    const windows = buildTwoWindowFixture();

    const saveResult = saveWindowLayout(windows, 0);
    expect(saveResult).toEqual({ status: "ok", revision: 1 });

    const loaded = loadWindowLayout();
    expect(loaded).not.toBeNull();
    expect(loaded!.version).toBe(2);
    expect(loaded!.revision).toBe(1);
    expect(loaded!.windows).toEqual(windows);
  });
});
