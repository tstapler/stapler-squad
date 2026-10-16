import { saveWindowLayout, getLastFocusedWindowId, setLastFocusedWindowId } from "../windowPersistence";
import type { NamedWindow, PersistedWindowLayoutV2 } from "../windowTypes";

const WINDOW_LAYOUT_KEY = "cockpit.windowLayout";
const LAST_FOCUSED_WINDOW_KEY = "cockpit.lastFocusedWindowId";

const SAMPLE_WINDOWS: NamedWindow[] = [
  {
    id: "win-1",
    name: "Window 1",
    paneState: {
      root: { type: "leaf", id: "p1", viewKind: "session-detail", sessionId: null, activeTab: "terminal" },
      focusedPaneId: "p1",
      zoomedPaneId: null,
    },
  },
];

beforeEach(() => {
  localStorage.clear();
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  jest.restoreAllMocks();
});

describe("saveWindowLayout revision guard", () => {
  it("windowPersistence_should_bumpRevisionAndWriteThrough_When_saveCalledWithCurrentLastKnownRevision", () => {
    const existing: PersistedWindowLayoutV2 = { version: 2, revision: 5, windows: SAMPLE_WINDOWS };
    localStorage.setItem(WINDOW_LAYOUT_KEY, JSON.stringify(existing));

    const newWindows: NamedWindow[] = [{ ...SAMPLE_WINDOWS[0], name: "Renamed" }];
    const result = saveWindowLayout(newWindows, 5);

    expect(result).toEqual({ status: "ok", revision: 6 });
    const stored = JSON.parse(localStorage.getItem(WINDOW_LAYOUT_KEY)!) as PersistedWindowLayoutV2;
    expect(stored).toEqual({ version: 2, revision: 6, windows: newWindows });
  });

  it("windowPersistence_should_rejectWriteAndReturnLatest_When_saveCalledWithStaleLastKnownRevision", () => {
    const existing: PersistedWindowLayoutV2 = { version: 2, revision: 5, windows: SAMPLE_WINDOWS };
    localStorage.setItem(WINDOW_LAYOUT_KEY, JSON.stringify(existing));

    const newWindows: NamedWindow[] = [{ ...SAMPLE_WINDOWS[0], name: "Should not land" }];
    const result = saveWindowLayout(newWindows, 4);

    expect(result).toEqual({ status: "conflict", latest: existing });
    // localStorage is unchanged — still revision 5, never overwritten.
    const stored = JSON.parse(localStorage.getItem(WINDOW_LAYOUT_KEY)!) as PersistedWindowLayoutV2;
    expect(stored).toEqual(existing);
    expect(console.error).toHaveBeenCalled();
  });
});

describe("lastFocusedWindowId bypasses the revision guard", () => {
  it("windowPersistence_should_writeAndReadLastFocusedWindowId_When_setThenGetCalled_bypassingRevisionGuard", () => {
    expect(getLastFocusedWindowId()).toBeNull();

    setLastFocusedWindowId("win-2");

    expect(localStorage.getItem(LAST_FOCUSED_WINDOW_KEY)).toBe("win-2");
    expect(getLastFocusedWindowId()).toBe("win-2");
  });
});
