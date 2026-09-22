import { CLOSED, listboxKey, type ListboxState } from "./useListboxNav";

const open = (active: number): ListboxState => ({ open: true, active });

describe("listboxKey", () => {
  it("useListboxNav_should_WrapOnDownUpAndCloseOnEscape_When_KeysPressed", () => {
    expect(listboxKey(open(-1), "ArrowDown", false, 3).state).toEqual(open(0));
    expect(listboxKey(open(2), "ArrowDown", false, 3).state).toEqual(open(0));
    expect(listboxKey(open(0), "ArrowUp", false, 3).state).toEqual(open(2));
    expect(listboxKey(open(-1), "ArrowUp", false, 3).state).toEqual(open(2));
    expect(listboxKey(open(1), "Escape", false, 3)).toEqual({ state: CLOSED, accept: null, handled: true });
    expect(listboxKey(CLOSED, "Escape", false, 3).handled).toBe(false);
  });

  it("useListboxNav_should_OpenWithoutActive_When_AltDown", () => {
    expect(listboxKey(CLOSED, "ArrowDown", true, 3).state).toEqual(open(-1));
    expect(listboxKey(CLOSED, "ArrowDown", false, 3).state).toEqual(open(0));
  });

  it("useListboxNav_should_AcceptOnEnterAndTab_Only_When_OptionActive", () => {
    expect(listboxKey(open(1), "Enter", false, 3)).toEqual({ state: CLOSED, accept: 1, handled: true });
    expect(listboxKey(open(1), "Tab", false, 3)).toEqual({ state: CLOSED, accept: 1, handled: true });
    expect(listboxKey(open(-1), "Enter", false, 3)).toMatchObject({ accept: null, handled: false });
    expect(listboxKey(open(-1), "Tab", false, 3)).toEqual({ state: CLOSED, accept: null, handled: false });
    expect(listboxKey(CLOSED, "Tab", false, 3).handled).toBe(false);
  });

  it("useListboxNav_should_IgnoreEverything_When_NoOptions", () => {
    expect(listboxKey(open(-1), "ArrowDown", false, 0).handled).toBe(false);
  });
});
