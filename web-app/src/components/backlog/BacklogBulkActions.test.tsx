import { render, screen, fireEvent } from "@testing-library/react";
import { BacklogBulkActions } from "./BacklogBulkActions";

jest.mock("@/lib/analytics", () => ({ useAnalytics: () => ({ track: jest.fn() }) }));

function renderToolbar(props: Partial<React.ComponentProps<typeof BacklogBulkActions>> = {}) {
  const handlers = {
    onArchiveSelected: jest.fn(),
    onDeleteSelected: jest.fn(),
    onTriageSelected: jest.fn(),
    onAutoImplementSelected: jest.fn(),
    onSelectAll: jest.fn(),
    onClear: jest.fn(),
  };
  render(<BacklogBulkActions selectedCount={0} totalCount={5} autoImplementEligibleIds={[]} {...handlers} {...props} />);
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

  describe("triage selected", () => {
    it("is shown, fires the handler, and is disabled while busy", () => {
      const h = renderToolbar({ selectedCount: 2 });
      fireEvent.click(screen.getByTestId("backlog-bulk-triage-selected"));
      expect(h.onTriageSelected).toHaveBeenCalledTimes(1);
    });

    it("is disabled when busy", () => {
      renderToolbar({ selectedCount: 2, busy: true });
      expect(screen.getByTestId("backlog-bulk-triage-selected")).toBeDisabled();
      expect(screen.getByTestId("backlog-bulk-auto-implement-selected")).toBeDisabled();
    });
  });

  describe("auto-implement selected", () => {
    it("opens a dialog with eligible/ineligible counts and autonomy warning, without side effects", () => {
      const h = renderToolbar({ selectedCount: 3, autoImplementEligibleIds: ["a", "b"] });
      expect(screen.queryByTestId("backlog-bulk-auto-implement-dialog")).toBeNull();
      fireEvent.click(screen.getByTestId("backlog-bulk-auto-implement-selected"));
      const dialog = screen.getByRole("dialog");
      expect(dialog).toHaveAttribute("aria-modal", "true");
      expect(dialog).toHaveTextContent("Start autonomous implementation for 2 items?");
      expect(dialog).toHaveTextContent("1 selected item is ineligible");
      expect(dialog).toHaveTextContent("without manual steps");
      expect(h.onAutoImplementSelected).not.toHaveBeenCalled();
    });

    it("Cancel closes without firing and returns focus to the trigger", () => {
      const h = renderToolbar({ selectedCount: 1, autoImplementEligibleIds: ["a"] });
      const trigger = screen.getByTestId("backlog-bulk-auto-implement-selected");
      trigger.focus();
      fireEvent.click(trigger);
      fireEvent.click(screen.getByText("Cancel"));
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(h.onAutoImplementSelected).not.toHaveBeenCalled();
      expect(trigger).toHaveFocus();
    });

    it("Escape closes without firing", () => {
      const h = renderToolbar({ selectedCount: 1, autoImplementEligibleIds: ["a"] });
      fireEvent.click(screen.getByTestId("backlog-bulk-auto-implement-selected"));
      fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(h.onAutoImplementSelected).not.toHaveBeenCalled();
    });

    it("Confirm fires once with the ids frozen at open time, even if the live list changes", () => {
      const handlers = { onAutoImplementSelected: jest.fn() };
      const base = { selectedCount: 2, totalCount: 5, onArchiveSelected: jest.fn(), onDeleteSelected: jest.fn(), onTriageSelected: jest.fn(), onSelectAll: jest.fn(), onClear: jest.fn(), ...handlers };
      const { rerender } = render(<BacklogBulkActions {...base} autoImplementEligibleIds={["a", "b"]} />);
      fireEvent.click(screen.getByTestId("backlog-bulk-auto-implement-selected"));
      rerender(<BacklogBulkActions {...base} autoImplementEligibleIds={["a"]} />);
      expect(screen.getByRole("dialog")).toHaveTextContent("for 2 items?");
      fireEvent.click(screen.getByTestId("backlog-bulk-auto-implement-confirm"));
      expect(handlers.onAutoImplementSelected).toHaveBeenCalledTimes(1);
      expect(handlers.onAutoImplementSelected).toHaveBeenCalledWith(["a", "b"]);
      expect(screen.queryByRole("dialog")).toBeNull();
    });

    it("drops the dialog when the selection empties and does not resurrect it", () => {
      const base = { totalCount: 5, onArchiveSelected: jest.fn(), onDeleteSelected: jest.fn(), onTriageSelected: jest.fn(), onAutoImplementSelected: jest.fn(), onSelectAll: jest.fn(), onClear: jest.fn() };
      const { rerender } = render(<BacklogBulkActions {...base} selectedCount={1} autoImplementEligibleIds={["a"]} />);
      fireEvent.click(screen.getByTestId("backlog-bulk-auto-implement-selected"));
      rerender(<BacklogBulkActions {...base} selectedCount={0} autoImplementEligibleIds={[]} />);
      rerender(<BacklogBulkActions {...base} selectedCount={1} autoImplementEligibleIds={["z"]} />);
      expect(screen.queryByRole("dialog")).toBeNull();
    });

    it("disables Confirm when nothing is eligible", () => {
      renderToolbar({ selectedCount: 2, autoImplementEligibleIds: [] });
      fireEvent.click(screen.getByTestId("backlog-bulk-auto-implement-selected"));
      expect(screen.getByTestId("backlog-bulk-auto-implement-confirm")).toBeDisabled();
    });
  });
});
