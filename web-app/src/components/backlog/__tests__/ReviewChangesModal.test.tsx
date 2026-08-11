/**
 * Tests for ReviewChangesModal's fetch-failure handling.
 *
 * getBacklogItemDiff failures previously collapsed into a fake
 * `{content: "", added: 0, removed: 0}` result, rendering identically to a
 * genuinely empty diff ("No changes to display") — exactly when a reviewer
 * most needs to notice the fetch actually failed
 * (docs/tasks/backlog-feature-improvement.md, Manual Gates section). Now a
 * failure must render DiffRenderer's distinct error state with a working
 * retry button.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ReviewChangesModal } from "../ReviewChangesModal";

const getBacklogItemDiff = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    getBacklogItemDiff: (...args: unknown[]) => getBacklogItemDiff(...args),
  }),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));

describe("ReviewChangesModal", () => {
  beforeEach(() => {
    getBacklogItemDiff.mockReset();
  });

  it("ReviewChangesModal_should_renderRetryableError_When_diffFetchRejects", async () => {
    getBacklogItemDiff.mockRejectedValue(new Error("server unavailable"));

    render(<ReviewChangesModal itemId="item-1" onClose={jest.fn()} />);

    await waitFor(() => expect(screen.getByText("Failed to load changes")).toBeInTheDocument());
    expect(screen.getByText("server unavailable")).toBeInTheDocument();
    expect(screen.queryByText("No changes to display")).toBeNull();
  });

  it("ReviewChangesModal_should_retryFetch_When_retryButtonClicked", async () => {
    getBacklogItemDiff.mockRejectedValueOnce(new Error("server unavailable"));
    getBacklogItemDiff.mockResolvedValueOnce({ diff: "", added: 0, removed: 0 });

    render(<ReviewChangesModal itemId="item-1" onClose={jest.fn()} />);

    await waitFor(() => expect(screen.getByText("Failed to load changes")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /retry/i }));

    await waitFor(() => expect(screen.getByText("No changes to display")).toBeInTheDocument());
    expect(screen.queryByText("Failed to load changes")).toBeNull();
    expect(getBacklogItemDiff).toHaveBeenCalledTimes(2);
  });

  it("ReviewChangesModal_should_renderGenuineEmptyState_When_fetchSucceedsWithNoDiff", async () => {
    getBacklogItemDiff.mockResolvedValue({ diff: "", added: 0, removed: 0 });

    render(<ReviewChangesModal itemId="item-1" onClose={jest.fn()} />);

    await waitFor(() => expect(screen.getByText("No changes to display")).toBeInTheDocument());
    expect(screen.queryByText("Failed to load changes")).toBeNull();
  });
});
