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
    notes: "",
    statusEvents: [],
    progressNotes: [],
    totalEstimatedCostUsd: 0,
    allowedTransitions: ["pr_pending", "done", "in_progress"],
    ...overrides,
  };
}

describe("ManualOverrideSection", () => {
  it("renders only the statuses the server reports in allowedTransitions", () => {
    render(
      <ManualOverrideSection
        item={makeItem({ allowedTransitions: ["pr_pending", "in_progress"] })}
        defaultExpanded={true}
        onOverrideStatus={jest.fn()}
        onAssociatePR={jest.fn()}
      />
    );

    const select = screen.getByTestId("manual-override-status-select") as HTMLSelectElement;
    const optionValues = Array.from(select.options).map((o) => o.value);
    expect(optionValues).toEqual(["", "pr_pending", "in_progress"]);
  });

  it("keeps the override submit button disabled until a status is chosen and the reason meets the minimum length", () => {
    render(
      <ManualOverrideSection
        item={makeItem()}
        defaultExpanded={true}
        onOverrideStatus={jest.fn()}
        onAssociatePR={jest.fn()}
      />
    );

    const submit = screen.getByTestId("manual-override-status-submit");
    expect(submit).toBeDisabled();

    fireEvent.change(screen.getByTestId("manual-override-status-select"), { target: { value: "done" } });
    expect(submit).toBeDisabled();

    fireEvent.change(screen.getByTestId("manual-override-reason-textarea"), { target: { value: "abc" } });
    expect(submit).toBeDisabled();

    fireEvent.change(screen.getByTestId("manual-override-reason-textarea"), { target: { value: "reviewer session zombied" } });
    expect(submit).not.toBeDisabled();
  });

  it("calls onOverrideStatus with the real reason text, not a hardcoded empty string", async () => {
    const onOverrideStatus = jest.fn().mockResolvedValue(undefined);
    render(
      <ManualOverrideSection
        item={makeItem()}
        defaultExpanded={true}
        onOverrideStatus={onOverrideStatus}
        onAssociatePR={jest.fn()}
      />
    );

    fireEvent.change(screen.getByTestId("manual-override-status-select"), { target: { value: "done" } });
    fireEvent.change(screen.getByTestId("manual-override-reason-textarea"), {
      target: { value: "reviewer session zombied, unsticking manually" },
    });
    fireEvent.click(screen.getByTestId("manual-override-status-submit"));

    await waitFor(() =>
      expect(onOverrideStatus).toHaveBeenCalledWith("done", "reviewer session zombied, unsticking manually")
    );
  });

  it("surfaces the server's rejection message on failure instead of a generic error", async () => {
    const onOverrideStatus = jest.fn().mockRejectedValue(new Error("someone else changed this — reload and try again"));
    render(
      <ManualOverrideSection
        item={makeItem()}
        defaultExpanded={true}
        onOverrideStatus={onOverrideStatus}
        onAssociatePR={jest.fn()}
      />
    );

    fireEvent.change(screen.getByTestId("manual-override-status-select"), { target: { value: "done" } });
    fireEvent.change(screen.getByTestId("manual-override-reason-textarea"), { target: { value: "forcing done" } });
    fireEvent.click(screen.getByTestId("manual-override-status-submit"));

    await waitFor(() => {
      expect(screen.getByRole("alert").textContent).toContain("someone else changed this — reload and try again");
    });
  });

  it("only renders the PR-association form while the item is in review status", () => {
    const { rerender } = render(
      <ManualOverrideSection
        item={makeItem({ status: "review" })}
        defaultExpanded={true}
        onOverrideStatus={jest.fn()}
        onAssociatePR={jest.fn()}
      />
    );
    expect(screen.getByTestId("manual-override-pr-url-input")).toBeInTheDocument();

    rerender(
      <ManualOverrideSection
        item={makeItem({ status: "in_progress" })}
        defaultExpanded={true}
        onOverrideStatus={jest.fn()}
        onAssociatePR={jest.fn()}
      />
    );
    expect(screen.queryByTestId("manual-override-pr-url-input")).not.toBeInTheDocument();
  });

  it("calls onAssociatePR with a parsed PR number", async () => {
    const onAssociatePR = jest.fn().mockResolvedValue(undefined);
    render(
      <ManualOverrideSection
        item={makeItem({ status: "review" })}
        defaultExpanded={true}
        onOverrideStatus={jest.fn()}
        onAssociatePR={onAssociatePR}
      />
    );

    fireEvent.change(screen.getByTestId("manual-override-pr-url-input"), {
      target: { value: "https://github.com/tstapler/stapler-squad/pull/320" },
    });
    fireEvent.change(screen.getByTestId("manual-override-pr-number-input"), { target: { value: "320" } });
    fireEvent.click(screen.getByTestId("manual-override-pr-submit"));

    await waitFor(() =>
      expect(onAssociatePR).toHaveBeenCalledWith("https://github.com/tstapler/stapler-squad/pull/320", 320)
    );
  });

  it("disables every write action when readOnly", () => {
    render(
      <ManualOverrideSection
        item={makeItem({ status: "review" })}
        defaultExpanded={true}
        readOnly={true}
        onOverrideStatus={jest.fn()}
        onAssociatePR={jest.fn()}
      />
    );

    expect(screen.getByTestId("manual-override-pr-submit")).toBeDisabled();
    expect(screen.getByTestId("manual-override-status-submit")).toBeDisabled();
  });
});
