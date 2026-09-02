import React from "react";
import { render, screen, waitFor, fireEvent, act } from "@testing-library/react";
import { VcsWidgetComments } from "./VcsWidgetComments";

const mockGetPRComments = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    getPRComments: mockGetPRComments,
  })),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: jest.fn(() => ({})),
}));

function renderWidget() {
  return render(
    <VcsWidgetComments owner="acme" repo="widget" prNumber={7} sessionId="session-1" />
  );
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
        { id: 1, author: "octocat", body: "Looks good", createdAt: undefined, isReview: false },
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
        { id: 1, author: "octocat", body: "Looks good", createdAt: undefined, isReview: false },
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

    fireEvent.click(screen.getByTestId("collapsible-header-pr-comments"));

    await waitFor(() => expect(screen.getByText("Failed to load comments")).toBeInTheDocument());
  });

  it("VcsWidgetComments_should_RenderViewOnGitHubLink_When_CommentIsGeneral", async () => {
    mockGetPRComments.mockResolvedValue({
      comments: [{ id: 42, author: "octocat", body: "General note", isReview: false }],
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
