import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { PollError } from "@/gen/session/v1/types_pb";
import { makePr } from "@/lib/pr/__testUtils__/pullRequests";
import { PullRequestChip } from "./PullRequestChip";

describe("PullRequestChip", () => {
  const three = [
    makePr({ number: 12, isActive: true, approvedCount: 1, checkConclusion: "success" }),
    makePr({ number: 11, state: "merged", mergedAt: 2 }),
    makePr({ number: 10, state: "merged", mergedAt: 1 }),
  ];

  it("chip_should_renderOneBadgeAndPlusTwo_When_SessionHasThreePrs", () => {
    render(<PullRequestChip pullRequests={three} onOpenList={jest.fn()} />);
    expect(screen.getAllByTestId("pr-chip")).toHaveLength(1);
    expect(screen.getByTestId("pr-more-button")).toHaveTextContent("+2");
  });

  it("badge_should_readViewGitHubPullRequest12AcmeAppStatusOpenOpensInNewTab", () => {
    render(<PullRequestChip pullRequests={three} onOpenList={jest.fn()} />);
    expect(
      screen.getByRole("link", { name: "View GitHub Pull Request #12, acme/app, status: open, opens in new tab" }),
    ).toBeInTheDocument();
  });

  it("more_button_should_haveTheDocumentedAccessibleName", () => {
    render(<PullRequestChip pullRequests={three} onOpenList={jest.fn()} />);
    expect(screen.getByRole("button", { name: "2 more pull requests, open pull request list" })).toBeInTheDocument();
  });

  it("badge_should_linkToGitHub_When_ListFormOmitsUrl", async () => {
    const open = jest.spyOn(window, "open").mockImplementation(() => null);
    render(<PullRequestChip pullRequests={[makePr({ number: 1470, isActive: true, url: "" })]} onOpenList={jest.fn()} />);
    const link = screen.getByTestId("pr-chip");
    expect(link).toHaveAttribute("href", "https://github.com/acme/app/pull/1470");
    await userEvent.click(link);
    expect(open).toHaveBeenCalledWith("https://github.com/acme/app/pull/1470", "_blank", "noopener,noreferrer");
    open.mockRestore();
  });

  it("chip_should_renderNoPlusN_When_OnlyOnePr", () => {
    render(<PullRequestChip pullRequests={[three[0]]} onOpenList={jest.fn()} />);
    expect(screen.queryByTestId("pr-more-button")).not.toBeInTheDocument();
  });

  it("chip_should_showLatestWithRealState_When_AllPrsTerminalAndNoneActive", () => {
    render(<PullRequestChip pullRequests={[three[1], three[2]]} onOpenList={jest.fn()} />);
    expect(screen.getByRole("link", { name: /#11, acme\/app, status: merged/ })).toBeInTheDocument();
    expect(screen.getByTestId("pr-more-button")).toHaveTextContent("+1");
  });

  it.each([
    ["merged", { state: "merged" }, "Merged", "merged"],
    ["closed", { state: "closed" }, "Closed", "closed"],
    ["open", { state: "open" }, "Open", "open"],
    ["draft", { state: "open", isDraft: true }, "Draft", "draft"],
    ["checking", { state: "unknown" }, "Checking", "unknown"],
  ])("chip_should_exposeGlyphShapeAndVisibleText_When_%s", (_n, over, text, glyph) => {
    const { container } = render(
      <PullRequestChip pullRequests={[makePr({ number: 5, isActive: true, ...over })]} onOpenList={jest.fn()} />,
    );
    expect(screen.getByTestId("pr-chip")).toHaveTextContent(text);
    expect(container.querySelector(`svg[data-glyph="${glyph}"]`)).not.toBeNull();
  });

  it("chip_should_keepTheMergedTextAndOfferATitle_When_CompactAndMerged", () => {
    render(<PullRequestChip compact pullRequests={[makePr({ number: 5, isActive: true, state: "merged" })]} onOpenList={jest.fn()} />);
    expect(screen.getByTestId("pr-chip")).toHaveAttribute("title", expect.stringContaining("State: merged"));
  });

  it("chip_should_markUnverifiedTerminalAsNotMerged", () => {
    render(
      <PullRequestChip
        pullRequests={[makePr({ number: 9, isActive: true, state: "unknown", outcomeUnverified: true })]}
        onOpenList={jest.fn()}
      />,
    );
    expect(screen.getByTestId("pr-chip")).toHaveAttribute("title", expect.stringContaining("Closed or merged (unverified)"));
    expect(screen.getByTestId("pr-chip")).not.toHaveTextContent("Merged");
  });

  it("chip_should_warnThatStatusMayBeOutOfDate_When_ActivePollFailed", () => {
    render(
      <PullRequestChip
        pullRequests={[makePr({ number: 5, isActive: true, pollError: PollError.AUTH })]}
        onOpenList={jest.fn()}
      />,
    );
    expect(screen.getByTestId("pr-chip")).toHaveAttribute("title", expect.stringContaining("Status may be out of date"));
    expect(screen.getByTestId("pr-chip")).toHaveAttribute("aria-label", expect.stringContaining("may be out of date"));
  });

  it("plusN_should_openTheListOnlyAndBadgeOpenGitHubOnly_When_InsideAClickableCard", async () => {
    const user = userEvent.setup();
    const cardClick = jest.fn();
    const openList = jest.fn();
    const open = jest.spyOn(window, "open").mockImplementation(() => null);
    render(
      <div onClick={cardClick} role="presentation">
        <PullRequestChip pullRequests={three} onOpenList={openList} />
      </div>,
    );
    await user.click(screen.getByTestId("pr-more-button"));
    expect(openList).toHaveBeenCalledTimes(1);
    expect(open).not.toHaveBeenCalled();
    expect(cardClick).not.toHaveBeenCalled();

    await user.click(screen.getByTestId("pr-chip"));
    expect(open).toHaveBeenCalledWith("https://github.com/acme/app/pull/12", "_blank", "noopener,noreferrer");
    expect(openList).toHaveBeenCalledTimes(1);
    expect(cardClick).not.toHaveBeenCalled();
    open.mockRestore();
  });

  it("plusN_should_beKeyboardOperable", async () => {
    const user = userEvent.setup();
    const openList = jest.fn();
    render(<PullRequestChip pullRequests={three} onOpenList={openList} />);
    screen.getByTestId("pr-more-button").focus();
    await user.keyboard("{Enter}");
    fireEvent.keyDown(screen.getByTestId("pr-more-button"), { key: " " });
    expect(openList).toHaveBeenCalled();
  });
});
