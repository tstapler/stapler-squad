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

describe("BacklogFilterBar archive done", () => {
  it("is absent without onArchiveDone", () => {
    renderBar();
    expect(screen.queryByTestId("backlog-archive-done-button")).toBeNull();
  });

  it("is disabled at zero and shows the count", () => {
    renderBar({ onArchiveDone: jest.fn(), doneCount: 0 });
    expect(screen.getByTestId("backlog-archive-done-button")).toBeDisabled();
    expect(screen.getByText("Archive done (0)")).toBeInTheDocument();
  });

  it("fires onArchiveDone when enabled", () => {
    const onArchiveDone = jest.fn();
    renderBar({ onArchiveDone, doneCount: 4 });
    fireEvent.click(screen.getByText("Archive done (4)"));
    expect(onArchiveDone).toHaveBeenCalledTimes(1);
  });
});

