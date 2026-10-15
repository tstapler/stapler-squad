import { render, screen } from "@testing-library/react";
import { createRef } from "react";
import { HistoryFilterBar } from "./HistoryFilterBar";

jest.mock("@/components/history/HistorySearchInput", () => ({ HistorySearchInput: () => <input aria-label="search" /> }));

const noop = jest.fn();

function renderBar() {
  const props = {
    searchQuery: "", branchFilter: "", selectedModel: "all", dateFilter: "all", sortField: "updated",
    sortOrder: "desc", groupingStrategy: "none", searchMode: "title",
    setSearchQuery: noop, setBranchFilter: noop, setSelectedModel: noop, setDateFilter: noop,
    setSortField: noop, setSortOrder: noop, setGroupingStrategy: noop, setSearchMode: noop,
    uniqueModels: ["opus"], hasActiveFilters: false, searching: false, onSearch: noop, onClearFilters: noop,
    searchInputRef: createRef<HTMLInputElement>(), fullTextSearch: {},
  };
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  return render(<HistoryFilterBar {...(props as any)} />);
}

describe("HistoryFilterBar accessibility", () => {
  it("gives every <select> an accessible name (axe select-name)", () => {
    renderBar();
    const selects = screen.getAllByRole("combobox");
    expect(selects.length).toBeGreaterThanOrEqual(4);
    for (const s of selects) {
      expect(s).toHaveAccessibleName();
    }
    for (const name of ["Filter by model", "Filter by date", "Sort by", "Group by"]) {
      expect(screen.getByRole("combobox", { name })).toBeInTheDocument();
    }
  });
});
