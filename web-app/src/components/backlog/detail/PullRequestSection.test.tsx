import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { PullRequestSection } from "./PullRequestSection";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Item",
    status: "pr_pending",
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
    ...overrides,
  };
}

describe("PullRequestSection", () => {
  it("renders the recorded PR link when item is pr_pending with a PR already recorded", () => {
    render(
      <PullRequestSection
        item={makeItem({ prUrl: "https://github.com/acme/widgets/pull/7", prNumber: 7 })}
        actionLoading={null}
        onMarkDone={jest.fn()}
      />
    );
    expect(screen.getByText(/PR #7/)).toBeInTheDocument();
    expect(screen.getByTestId("backlog-action-mark-done")).toBeInTheDocument();
  });

  it("does not render the Link existing PR control when the item is pr_pending (already has a PR)", () => {
    render(
      <PullRequestSection
        item={makeItem({ prUrl: "https://github.com/acme/widgets/pull/7", prNumber: 7 })}
        actionLoading={null}
        onMarkDone={jest.fn()}
        onLinkPr={jest.fn()}
      />
    );
    expect(screen.queryByTestId("backlog-link-existing-pr-toggle")).not.toBeInTheDocument();
  });

  it("renders the Link existing PR control for a review-status item with no PR yet, when onLinkPr is provided", () => {
    render(
      <PullRequestSection
        item={makeItem({ status: "review", prUrl: undefined })}
        actionLoading={null}
        onMarkDone={jest.fn()}
        onLinkPr={jest.fn()}
      />
    );
    expect(screen.getByTestId("backlog-link-existing-pr-toggle")).toBeInTheDocument();
  });

  it("does not render the Link existing PR control when onLinkPr is omitted, even in review status", () => {
    render(
      <PullRequestSection
        item={makeItem({ status: "review", prUrl: undefined })}
        actionLoading={null}
        onMarkDone={jest.fn()}
      />
    );
    expect(screen.queryByTestId("backlog-link-existing-pr-toggle")).not.toBeInTheDocument();
  });

  it("keeps the Link existing PR submit disabled until both a URL and a valid PR number are entered", () => {
    render(
      <PullRequestSection
        item={makeItem({ status: "review", prUrl: undefined })}
        actionLoading={null}
        onMarkDone={jest.fn()}
        onLinkPr={jest.fn()}
      />
    );
    fireEvent.click(screen.getByTestId("backlog-link-existing-pr-toggle"));

    const submit = screen.getByTestId("backlog-link-existing-pr-submit");
    expect(submit).toBeDisabled();

    fireEvent.change(screen.getByTestId("backlog-link-existing-pr-url"), {
      target: { value: "https://github.com/acme/widgets/pull/42" },
    });
    expect(submit).toBeDisabled();

    fireEvent.change(screen.getByTestId("backlog-link-existing-pr-number"), { target: { value: "42" } });
    expect(submit).not.toBeDisabled();
  });

  it("calls onLinkPr with the URL and parsed number, then resets the form on success", async () => {
    const onLinkPr = jest.fn().mockResolvedValue(undefined);
    render(
      <PullRequestSection
        item={makeItem({ status: "review", prUrl: undefined })}
        actionLoading={null}
        onMarkDone={jest.fn()}
        onLinkPr={onLinkPr}
      />
    );
    fireEvent.click(screen.getByTestId("backlog-link-existing-pr-toggle"));
    fireEvent.change(screen.getByTestId("backlog-link-existing-pr-url"), {
      target: { value: "https://github.com/acme/widgets/pull/42" },
    });
    fireEvent.change(screen.getByTestId("backlog-link-existing-pr-number"), { target: { value: "42" } });
    fireEvent.click(screen.getByTestId("backlog-link-existing-pr-submit"));

    await waitFor(() =>
      expect(onLinkPr).toHaveBeenCalledWith("https://github.com/acme/widgets/pull/42", 42)
    );
    await waitFor(() =>
      expect(screen.queryByTestId("backlog-link-existing-pr-form")).not.toBeInTheDocument()
    );
  });

  it("surfaces the server's rejection message on failure (e.g. wrong status) instead of silently succeeding", async () => {
    const onLinkPr = jest.fn().mockRejectedValue(new Error("item is not in review status"));
    render(
      <PullRequestSection
        item={makeItem({ status: "review", prUrl: undefined })}
        actionLoading={null}
        onMarkDone={jest.fn()}
        onLinkPr={onLinkPr}
      />
    );
    fireEvent.click(screen.getByTestId("backlog-link-existing-pr-toggle"));
    fireEvent.change(screen.getByTestId("backlog-link-existing-pr-url"), {
      target: { value: "https://github.com/acme/widgets/pull/42" },
    });
    fireEvent.change(screen.getByTestId("backlog-link-existing-pr-number"), { target: { value: "42" } });
    fireEvent.click(screen.getByTestId("backlog-link-existing-pr-submit"));

    await waitFor(() =>
      expect(screen.getByTestId("backlog-link-existing-pr-error")).toHaveTextContent("item is not in review status")
    );
  });
});
