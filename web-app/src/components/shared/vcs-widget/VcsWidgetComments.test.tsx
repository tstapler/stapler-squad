import React from "react";
import { render, screen, waitFor, fireEvent, act } from "@testing-library/react";
import { VcsWidgetComments } from "./VcsWidgetComments";
import { rateLimitError } from "@/lib/vcs/__testUtils__/rateLimitFixtures";

const mockGetPRComments = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    getPRComments: mockGetPRComments,
  })),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: jest.fn(() => ({})),
}));

jest.mock("@/lib/contexts/AnalyticsContext", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

function renderWidget() {
  return render(
    <VcsWidgetComments owner="acme" repo="widget" prNumber={7} sessionId="session-1" />
  );
}

async function expandAndAwaitError(expectedText: string | RegExp) {
  fireEvent.click(screen.getByTestId("collapsible-header-pr-comments"));
  await waitFor(() => expect(screen.getByText(expectedText)).toBeInTheDocument());
}

describe("VcsWidgetComments", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("VcsWidgetComments_should_StartCollapsedAndMakeNoRpcCall_When_Rendered", () => {
    mockGetPRComments.mockResolvedValue({ comments: [] });
    renderWidget();

    expect(screen.getByTestId("collapsible-header-pr-comments")).toHaveAttribute(
      "aria-expanded",
      "false"
    );
    expect(mockGetPRComments).not.toHaveBeenCalled();
  });

  it("VcsWidgetComments_should_FetchExactlyOnce_When_ExpandedForTheFirstTime", async () => {
    mockGetPRComments.mockResolvedValue({
      comments: [
        { id: 1n, author: "octocat", body: "Looks good", createdAt: undefined, isReview: false },
      ],
    });
    renderWidget();

    fireEvent.click(screen.getByTestId("collapsible-header-pr-comments"));

    await waitFor(() => expect(screen.getByText("octocat")).toBeInTheDocument());
    expect(mockGetPRComments).toHaveBeenCalledTimes(1);
    expect(mockGetPRComments).toHaveBeenCalledWith({ id: "session-1" });
  });

  it("VcsWidgetComments_should_NotRefetch_When_ReCollapsedAndReExpanded", async () => {
    mockGetPRComments.mockResolvedValue({
      comments: [
        { id: 1n, author: "octocat", body: "Looks good", createdAt: undefined, isReview: false },
      ],
    });
    renderWidget();

    const header = screen.getByTestId("collapsible-header-pr-comments");

    fireEvent.click(header); // expand
    await waitFor(() => expect(screen.getByText("octocat")).toBeInTheDocument());
    expect(mockGetPRComments).toHaveBeenCalledTimes(1);

    fireEvent.click(header); // collapse
    expect(screen.queryByText("octocat")).not.toBeInTheDocument();

    fireEvent.click(header); // re-expand
    await waitFor(() => expect(screen.getByText("octocat")).toBeInTheDocument());
    expect(mockGetPRComments).toHaveBeenCalledTimes(1);
  });

  it("VcsWidgetComments_should_ShowLoadingText_When_FetchInFlight", async () => {
    let resolveFetch: (value: { comments: never[] }) => void = () => {};
    mockGetPRComments.mockReturnValue(
      new Promise((resolve) => {
        resolveFetch = resolve;
      })
    );
    renderWidget();

    fireEvent.click(screen.getByTestId("collapsible-header-pr-comments"));

    expect(screen.getByText("Loading…")).toBeInTheDocument();

    await act(async () => {
      resolveFetch({ comments: [] });
    });
  });

  it("VcsWidgetComments_should_ShowErrorText_When_FetchRejects", async () => {
    mockGetPRComments.mockRejectedValue(new Error("boom"));
    renderWidget();

    await expandAndAwaitError("Failed to load comments");
  });

  it("VcsWidgetComments_should_RenderRetryAsRealButton_When_FetchRejects", async () => {
    mockGetPRComments.mockRejectedValue(new Error("boom"));
    renderWidget();

    await expandAndAwaitError("Failed to load comments");

    const retryButton = screen.getByRole("button", { name: "Retry" });
    expect(retryButton.tagName).toBe("BUTTON");
  });

  it("VcsWidgetComments_should_ResetAndRefetch_When_RetryButtonActivated", async () => {
    mockGetPRComments
      .mockRejectedValueOnce(new Error("boom"))
      .mockResolvedValueOnce({
        comments: [{ id: 1n, author: "octocat", body: "Looks good", createdAt: undefined, isReview: false }],
      });
    renderWidget();

    await expandAndAwaitError("Failed to load comments");
    expect(mockGetPRComments).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => expect(screen.getByText("octocat")).toBeInTheDocument());
    expect(mockGetPRComments).toHaveBeenCalledTimes(2);
  });

  it("VcsWidgetComments_should_RenderErrorContainerAsPoliteLiveRegion_When_FetchRejects", async () => {
    mockGetPRComments.mockRejectedValue(new Error("boom"));
    renderWidget();

    await expandAndAwaitError("Failed to load comments");

    const errorBox = screen.getByRole("status");
    expect(errorBox).toHaveAttribute("aria-live", "polite");
  });

  it("VcsWidgetComments_should_RenderFriendlyRateLimitCopy_When_ErrorCarriesReasonMarker", async () => {
    mockGetPRComments.mockRejectedValue(rateLimitError("exhausted", 4 * 60_000));
    renderWidget();

    await expandAndAwaitError(/^GitHub rate limit reached — try again in ~.+\.$/);

    expect(screen.queryByText("Failed to load comments")).not.toBeInTheDocument();
  });

  it("VcsWidgetComments_should_RenderNoAutoRetryTransientCopy_When_ErrorCarriesTransientReasonMarker", async () => {
    mockGetPRComments.mockRejectedValue(rateLimitError("transient", 20_000));
    renderWidget();

    await expandAndAwaitError("GitHub is rate-limited right now — tap Retry to try again.");
  });

  it("VcsWidgetComments_should_RenderViewOnGitHubLink_When_CommentIsGeneral", async () => {
    mockGetPRComments.mockResolvedValue({
      comments: [{ id: 42n, author: "octocat", body: "General note", isReview: false }],
    });
    renderWidget();

    fireEvent.click(screen.getByTestId("collapsible-header-pr-comments"));

    await waitFor(() => expect(screen.getByText("View on GitHub ↗")).toBeInTheDocument());
    expect(screen.getByText("View on GitHub ↗")).toHaveAttribute(
      "href",
      "https://github.com/acme/widget/pull/7#issuecomment-42"
    );
  });
});
