import reducer, { toggleSelection, selectAll, clearSelection } from "./backlogBulkSelectionSlice";

const initial = { selectedIds: [] as string[], selectMode: false };

describe("backlogBulkSelectionSlice", () => {
  it("toggleSelection adds an id and enters select mode", () => {
    const state = reducer(initial, toggleSelection("a"));
    expect(state.selectedIds).toEqual(["a"]);
    expect(state.selectMode).toBe(true);
  });

  it("toggleSelection removes an already-selected id", () => {
    const state = reducer(reducer(initial, toggleSelection("a")), toggleSelection("a"));
    expect(state.selectedIds).toEqual([]);
  });

  it("selectAll replaces the selection", () => {
    const state = reducer(reducer(initial, toggleSelection("a")), selectAll(["b", "c"]));
    expect(state.selectedIds).toEqual(["b", "c"]);
    expect(state.selectMode).toBe(true);
  });

  it("clearSelection empties selection and leaves select mode", () => {
    const state = reducer(reducer(initial, selectAll(["x", "y"])), clearSelection());
    expect(state).toEqual(initial);
  });
});
