import { render, screen, fireEvent } from "@testing-library/react";
import { BacklogBulkActions } from "./BacklogBulkActions";

jest.mock("@/lib/analytics", () => ({ useAnalytics: () => ({ track: jest.fn() }) }));

function renderToolbar(props: Partial<React.ComponentProps<typeof BacklogBulkActions>> = {}) {
  const handlers = {
    onArchiveSelected: jest.fn(),
    onDeleteSelected: jest.fn(),
    onSelectAll: jest.fn(),
    onClear: jest.fn(),
  };
  render(<BacklogBulkActions selectedCount={0} totalCount={5} {...handlers} {...props} />);
  return handlers;
}

describe("BacklogBulkActions", () => {
  it("renders nothing for 0 selected", () => {
    renderToolbar();
    expect(screen.queryByTestId("backlog-bulk-actions")).toBeNull();
  });

  it("shows the feedback string alone when nothing is selected", () => {
    renderToolbar({ feedback: "Archived 3 items" });
    expect(screen.getByText("Archived 3 items")).toBeInTheDocument();
    expect(screen.queryByTestId("backlog-bulk-actions")).toBeNull();
  });

  it("renders the toolbar for 1 selected", () => {
    renderToolbar({ selectedCount: 1 });
    expect(screen.getByText("1 of 5 selected")).toBeInTheDocument();
    expect(screen.getByText("Archive selected")).toBeInTheDocument();
    expect(screen.getByText("Delete selected")).toBeInTheDocument();
    expect(screen.getByText("Select all")).toBeInTheDocument();
    expect(screen.getByText("Clear")).toBeInTheDocument();
  });

  it("renders the toolbar for N selected and wires the handlers", () => {
    const h = renderToolbar({ selectedCount: 3 });
    expect(screen.getByText("3 of 5 selected")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Archive selected"));
    fireEvent.click(screen.getByText("Select all"));
    fireEvent.click(screen.getByText("Clear"));
    expect(h.onArchiveSelected).toHaveBeenCalledTimes(1);
    expect(h.onSelectAll).toHaveBeenCalledTimes(1);
    expect(h.onClear).toHaveBeenCalledTimes(1);
  });

  it("delete asks for confirmation with the count before firing", () => {
    const h = renderToolbar({ selectedCount: 2 });
    fireEvent.click(screen.getByTestId("backlog-bulk-delete-selected"));
    expect(h.onDeleteSelected).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toHaveTextContent("Delete 2 items?");
    fireEvent.click(screen.getByTestId("backlog-bulk-delete-confirm"));
    expect(h.onDeleteSelected).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("cancel closes the dialog without deleting", () => {
    const h = renderToolbar({ selectedCount: 2 });
    fireEvent.click(screen.getByTestId("backlog-bulk-delete-selected"));
    fireEvent.click(screen.getByText("Cancel"));
    expect(h.onDeleteSelected).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
