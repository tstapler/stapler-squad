import reducer, {
  toggleSelection,
  selectAll,
  clearSelection,
} from "./bulkSelectionSlice";

const initial = { selectedIds: [] as string[] };

describe("bulkSelectionSlice", () => {
  it("toggleSelection adds an id that is not selected", () => {
    const state = reducer(initial, toggleSelection("a"));
    expect(state.selectedIds).toContain("a");
  });

  it("toggleSelection removes an id that is already selected", () => {
    const withA = reducer(initial, toggleSelection("a"));
    const state = reducer(withA, toggleSelection("a"));
    expect(state.selectedIds).not.toContain("a");
  });

  it("selectAll replaces selection with the provided ids", () => {
    const withA = reducer(initial, toggleSelection("a"));
    const state = reducer(withA, selectAll(["b", "c"]));
    expect(state.selectedIds).toEqual(["b", "c"]);
    expect(state.selectedIds).not.toContain("a");
  });

  it("clearSelection empties the selection", () => {
    const withItems = reducer(initial, selectAll(["x", "y"]));
    const state = reducer(withItems, clearSelection());
    expect(state.selectedIds).toHaveLength(0);
  });
});
