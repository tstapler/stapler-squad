import { renderHook } from "@testing-library/react";
import { useTrayDismissal } from "../useTrayDismissal";

function setup(isOpen: boolean) {
  const trayRef = { current: document.createElement("div") };
  return renderHook(
    ({ open }) =>
      useTrayDismissal({ enabled: true, isOpen: open, isSheet: true, modal: false, trayRef, close: jest.fn() }),
    { initialProps: { open: isOpen } },
  );
}

describe("useTrayDismissal history entry", () => {
  let back: jest.SpyInstance;
  beforeEach(() => {
    back = jest.spyOn(window.history, "back").mockImplementation(() => {});
  });
  afterEach(() => back.mockRestore());

  it("closing_a_sheet_without_navigation_should_pop_the_entry_it_pushed", () => {
    const { rerender } = setup(true);
    rerender({ open: false });
    expect(back).toHaveBeenCalledTimes(1);
  });

  it("closing_a_sheet_by_navigation_should_keep_history_untouched_after_release", () => {
    const hook = setup(true);
    hook.result.current.releaseHistoryEntry();
    hook.rerender({ open: false });
    expect(back).not.toHaveBeenCalled();
  });
});
