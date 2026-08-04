import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ManualOverrideSection } from "./ManualOverrideSection";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Item",
    status: "review",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    statusEvents: [],
    progressNotes: [],
    totalEstimatedCostUsd: 0,
    allowedTransitions: ["in_progress", "done", "pr_pending"],
    ...overrides,
  };
}

describe("ManualOverrideSection", () => {
  it("renders the toggle collapsed by default and reveals the form on click", () => {
    render(<ManualOverrideSection item={makeItem()} actionLoading={null} defaultExpanded={true} onOverride={jest.fn()} />);

    expect(screen.queryByTestId("backlog-manual-override-form")).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId("backlog-manual-override-toggle"));
    expect(screen.getByTestId("backlog-manual-override-form")).toBeInTheDocument();
  });

  it("populates the status <select> from item.allowedTransitions, not a re-encoded transition table", () => {
    render(
      <ManualOverrideSection
        item={makeItem({ allowedTransitions: ["in_progress", "archived"] })}
        actionLoading={null}
        defaultExpanded={true}
        onOverride={jest.fn()}
      />
    );
    fireEvent.click(screen.getByTestId("backlog-manual-override-toggle"));

    const options = screen
      .getByTestId("backlog-manual-override-status")
      .querySelectorAll("option")
      .values();
    const values = Array.from(options)
      .map((o) => (o as HTMLOptionElement).value)
      .filter(Boolean);
    expect(values).toEqual(["in_progress", "archived"]);
  });

  it("disables the toggle when the item has no allowed transitions", () => {
    render(
      <ManualOverrideSection
        item={makeItem({ allowedTransitions: [] })}
        actionLoading={null}
        defaultExpanded={true}
        onOverride={jest.fn()}
      />
    );
    expect(screen.getByTestId("backlog-manual-override-toggle")).toBeDisabled();
  });

  it("disables the toggle when readOnly is set, even with allowed transitions available", () => {
    render(
      <ManualOverrideSection
        item={makeItem()}
        actionLoading={null}
        defaultExpanded={true}
        readOnly={true}
        onOverride={jest.fn()}
      />
    );
    expect(screen.getByTestId("backlog-manual-override-toggle")).toBeDisabled();
  });

  it("keeps submit disabled until a status is chosen and the reason meets the minimum length", () => {
    render(
      <ManualOverrideSection item={makeItem()} actionLoading={null} defaultExpanded={true} onOverride={jest.fn()} />
    );
    fireEvent.click(screen.getByTestId("backlog-manual-override-toggle"));

    const submit = screen.getByTestId("backlog-manual-override-submit");
    expect(submit).toBeDisabled();

    fireEvent.change(screen.getByTestId("backlog-manual-override-status"), { target: { value: "done" } });
    expect(submit).toBeDisabled();

    fireEvent.change(screen.getByTestId("backlog-manual-override-reason"), { target: { value: "hi" } });
    expect(submit).toBeDisabled();

    fireEvent.change(screen.getByTestId("backlog-manual-override-reason"), {
      target: { value: "A sufficiently long override reason." },
    });
    expect(submit).not.toBeDisabled();
  });

  it("calls onOverride with the chosen status and trimmed reason, then resets the form on success", async () => {
    const onOverride = jest.fn().mockResolvedValue(undefined);
    render(
      <ManualOverrideSection item={makeItem()} actionLoading={null} defaultExpanded={true} onOverride={onOverride} />
    );
    fireEvent.click(screen.getByTestId("backlog-manual-override-toggle"));
    fireEvent.change(screen.getByTestId("backlog-manual-override-status"), { target: { value: "done" } });
    fireEvent.change(screen.getByTestId("backlog-manual-override-reason"), {
      target: { value: "  Recovering a wedged item.  " },
    });
    fireEvent.click(screen.getByTestId("backlog-manual-override-submit"));

    await waitFor(() => expect(onOverride).toHaveBeenCalledWith("done", "Recovering a wedged item."));
    await waitFor(() => expect(screen.queryByTestId("backlog-manual-override-form")).not.toBeInTheDocument());
  });

  it("surfaces the server's rejection message on failure instead of silently succeeding", async () => {
    const onOverride = jest.fn().mockRejectedValue(new Error("item is not in review status"));
    render(
      <ManualOverrideSection item={makeItem()} actionLoading={null} defaultExpanded={true} onOverride={onOverride} />
    );
    fireEvent.click(screen.getByTestId("backlog-manual-override-toggle"));
    fireEvent.change(screen.getByTestId("backlog-manual-override-status"), { target: { value: "done" } });
    fireEvent.change(screen.getByTestId("backlog-manual-override-reason"), {
      target: { value: "Recovering a wedged item." },
    });
    fireEvent.click(screen.getByTestId("backlog-manual-override-submit"));

    await waitFor(() =>
      expect(screen.getByTestId("backlog-manual-override-error")).toHaveTextContent("item is not in review status")
    );
    // Form stays open with the user's input intact so they can retry.
    expect(screen.getByTestId("backlog-manual-override-form")).toBeInTheDocument();
  });
});
