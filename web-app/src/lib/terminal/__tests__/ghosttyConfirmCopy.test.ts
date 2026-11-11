import { disableGhosttyAutoCopy, notifyGhosttySelectionCleared } from "../ghosttyConfirmCopy";

describe("disableGhosttyAutoCopy", () => {
  it("replaces the selection manager's copy with a no-op", async () => {
    const copied: string[] = [];
    const term = { selectionManager: { copyToClipboard: async (t: string) => void copied.push(t) } };

    expect(disableGhosttyAutoCopy(term)).toBe(true);
    await term.selectionManager.copyToClipboard("selected");

    expect(copied).toEqual([]);
  });

  it("reports false, without throwing, when the private API is gone", () => {
    expect(disableGhosttyAutoCopy({})).toBe(false);
    expect(disableGhosttyAutoCopy({ selectionManager: {} })).toBe(false);
  });
});

/** Mirrors ghostty-web's SelectionManager: clearSelection() clears silently, only the emitter notifies. */
function fakeSelectionManager() {
  const listeners: Array<() => void> = [];
  let selected = true;
  return {
    listeners,
    manager: {
      hasSelection: () => selected,
      clearSelection() {
        selected = false;
      },
      selectionChangedEmitter: { fire: () => listeners.forEach((l) => l()) },
    },
  };
}

describe("notifyGhosttySelectionCleared", () => {
  it("is needed: upstream clearSelection() does not notify", () => {
    const { listeners, manager } = fakeSelectionManager();
    const fired = jest.fn();
    listeners.push(fired);
    manager.clearSelection();
    expect(fired).not.toHaveBeenCalled();
  });

  it("fires the selection-change event when a clear removes a selection", () => {
    const { listeners, manager } = fakeSelectionManager();
    const fired = jest.fn();
    listeners.push(fired);
    expect(notifyGhosttySelectionCleared({ selectionManager: manager })).toBe(true);
    manager.clearSelection();
    expect(fired).toHaveBeenCalledTimes(1);
  });

  it("stays silent when there was no selection, and is idempotent", () => {
    const { listeners, manager } = fakeSelectionManager();
    const fired = jest.fn();
    listeners.push(fired);
    const term = { selectionManager: manager };
    notifyGhosttySelectionCleared(term);
    notifyGhosttySelectionCleared(term);
    manager.clearSelection();
    manager.clearSelection();
    expect(fired).toHaveBeenCalledTimes(1);
  });

  it("reports false when the private API is gone", () => {
    expect(notifyGhosttySelectionCleared({})).toBe(false);
    expect(notifyGhosttySelectionCleared({ selectionManager: {} })).toBe(false);
  });
});
