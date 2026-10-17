import { render, screen, fireEvent } from "@testing-library/react";
import { BacklogFilterBar } from "./BacklogFilterBar";

jest.mock("@/lib/analytics", () => ({ useAnalytics: () => ({ track: jest.fn() }) }));

function renderBar(props: Partial<React.ComponentProps<typeof BacklogFilterBar>> = {}) {
  return render(
    <BacklogFilterBar
      search=""
      onSearchChange={jest.fn()}
      statusFilter={[]}
      onStatusFilterChange={jest.fn()}
      priorityFilter={[]}
      onPriorityFilterChange={jest.fn()}
      showArchived={false}
      onShowArchivedChange={jest.fn()}
      onResetView={jest.fn()}
      showSortGroupControls={false}
      {...props}
    />
  );
}

describe("BacklogFilterBar source filter", () => {
  it("selects a source when its chip is clicked", () => {
    const onSourceFilterChange = jest.fn();
    renderBar({ sourceFilter: [], onSourceFilterChange });
    fireEvent.click(screen.getByTestId("backlog-filter-source-linear"));
    expect(onSourceFilterChange).toHaveBeenCalledWith(["linear"]);
  });

  it("deselects an already-selected source", () => {
    const onSourceFilterChange = jest.fn();
    renderBar({ sourceFilter: ["linear", "jira"], onSourceFilterChange });
    fireEvent.click(screen.getByTestId("backlog-filter-source-linear"));
    expect(onSourceFilterChange).toHaveBeenCalledWith(["jira"]);
  });

  it("renders no source chips when the handler is not provided", () => {
    renderBar();
    expect(screen.queryByLabelText("Filter by issue source")).toBeNull();
  });
});
