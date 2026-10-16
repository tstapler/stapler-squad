import {
  createScrollSettings,
  SCROLL_OVERRIDE_KEY,
  GESTURE_SCROLL_KEY,
  type ScrollSettingsStore,
} from "../scrollOverride";

const noopLog = () => {};

function make(extra: Parameters<typeof createScrollSettings>[0] = {}): ScrollSettingsStore {
  return createScrollSettings({ onOverrideChange: noopLog, ...extra });
}

beforeEach(() => localStorage.clear());

describe("scrollOverride persistence", () => {
  it("scrollOverride_should_PersistAndRestore_When_SetToTui", () => {
    make().setOverride("tui");
    expect(localStorage.getItem(SCROLL_OVERRIDE_KEY)).toBe("tui");
    expect(make().getOverride()).toBe("tui"); // a fresh store (reload) restores it
  });

  it("scrollOverride_should_FallBackToAuto_When_StoredValueInvalid", () => {
    localStorage.setItem(SCROLL_OVERRIDE_KEY, "sideways");
    expect(make().getOverride()).toBe("auto");
  });

  it("scrollOverride_should_RemoveKey_When_SetToAuto", () => {
    const store = make();
    store.setOverride("local");
    store.setOverride("auto");
    expect(localStorage.getItem(SCROLL_OVERRIDE_KEY)).toBeNull();
    expect(store.getOverride()).toBe("auto");
  });

  it("scrollOverride_should_FallBackToDefaults_When_LocalStorageThrows", () => {
    const boom = () => {
      throw new Error("storage denied");
    };
    const store = make({ getStorage: () => ({ getItem: boom, setItem: boom, removeItem: boom }) });
    expect(store.getOverride()).toBe("auto");
    expect(store.getGestureScroll()).toBe(true);
    expect(() => store.setOverride("tui")).not.toThrow();
    expect(() => store.setGestureScroll(false)).not.toThrow();
    // The in-memory choice still applies for the session.
    expect(store.getOverride()).toBe("tui");
    expect(store.getGestureScroll()).toBe(false);
  });

  it("scrollOverride_should_NotifySubscribersAndLogOnlyOnChange", () => {
    const log = jest.fn();
    const store = createScrollSettings({ onOverrideChange: log });
    const listener = jest.fn();
    const unsubscribe = store.subscribe(listener);
    store.setOverride("tui");
    store.setOverride("tui");
    expect(listener).toHaveBeenCalledTimes(1);
    expect(log).toHaveBeenCalledWith("auto", "tui");
    unsubscribe();
    store.setOverride("local");
    expect(listener).toHaveBeenCalledTimes(1);
  });
});

describe("gesture scroll persistence", () => {
  it("gestureScroll_should_PersistAndDefaultOn_When_Unset", () => {
    const store = make();
    expect(store.getGestureScroll()).toBe(true);
    store.setGestureScroll(false);
    expect(localStorage.getItem(GESTURE_SCROLL_KEY)).toBe("off");
    expect(make().getGestureScroll()).toBe(false);
    store.setGestureScroll(true);
    expect(localStorage.getItem(GESTURE_SCROLL_KEY)).toBeNull();
    expect(make().getGestureScroll()).toBe(true);
  });

  it("gestureScroll_should_FallBackToOn_When_StoredValueInvalidOrStorageThrows", () => {
    localStorage.setItem(GESTURE_SCROLL_KEY, "maybe");
    expect(make().getGestureScroll()).toBe(true);
    const boom = () => {
      throw new Error("storage denied");
    };
    expect(make({ getStorage: () => ({ getItem: boom, setItem: boom, removeItem: boom }) }).getGestureScroll()).toBe(true);
  });
});
