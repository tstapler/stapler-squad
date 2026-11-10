import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as fs from "fs";
import * as path from "path";
import { useState } from "react";
import { PollError, PrHold_Kind, type PrHold, type SessionPullRequest } from "@/gen/session/v1/types_pb";
import { timestampFromMs } from "@bufbuild/protobuf/wkt";
import { failure, OK, type PrActionResult, type PullRequestActions } from "@/lib/pr/actionResult";
import { makeHold, makePr } from "@/lib/pr/__testUtils__/pullRequests";
import { PullRequestList, type PullRequestListProps } from "./PullRequestList";

const NOW = Date.UTC(2026, 10, 5, 12, 0, 0);
const DAY = 86_400_000;
const merged = (n: number, daysAgo: number) => makePr({ number: n, state: "merged", mergedAt: NOW - daysAgo * DAY });

function fakeActions(over: Partial<PullRequestActions> = {}): jest.Mocked<PullRequestActions> {
  return {
    refresh: jest.fn(async () => OK),
    merge: jest.fn(async () => OK),
    close: jest.fn(async () => OK),
    askToFix: jest.fn(async () => OK),
    pin: jest.fn(async () => OK),
    detach: jest.fn(async () => OK),
    attachByUrl: jest.fn(async () => OK),
    ...over,
  } as jest.Mocked<PullRequestActions>;
}

/** Holds the list state so an action that removes a row re-renders like the real hook. */
function Harness({ initial, actions, ...rest }: { initial: SessionPullRequest[]; actions: PullRequestActions } & Partial<PullRequestListProps>) {
  const [prs, setPrs] = useState(initial);
  const wired: PullRequestActions = {
    ...actions,
    detach: async (t) => {
      const result = await actions.detach(t);
      if (result.ok) setPrs((cur) => cur.filter((p) => p.number !== t.number));
      return result;
    },
  };
  return <PullRequestList pullRequests={prs} actions={wired} nowMs={NOW} {...rest} />;
}

const three = () => [makePr({ number: 12, isActive: true }), merged(11, 2), merged(10, 4)];

function setPhone(matches: boolean) {
  window.matchMedia = jest.fn().mockImplementation((query: string) => ({
    matches: query.includes("max-width: 480px") ? matches : false,
    media: query,
    addEventListener: jest.fn(),
    removeEventListener: jest.fn(),
  })) as unknown as typeof window.matchMedia;
}

beforeEach(() => setPhone(false));

const setClipboard = (value: unknown) => Object.defineProperty(navigator, "clipboard", { value, configurable: true, writable: true });

describe("PullRequestList rows and disclosure", () => {
  it("list_should_showThreeRowsWithStateTextAndDefaultTargetHint_When_ThreePrs", () => {
    render(<PullRequestList pullRequests={three()} actions={fakeActions()} nowMs={NOW} />);
    expect(screen.getAllByRole("listitem")).toHaveLength(3);
    expect(screen.getByTestId("pr-row-12")).toHaveTextContent("Open");
    expect(screen.getByTestId("pr-row-11")).toHaveTextContent("Merged 2d");
    expect(screen.getByText("Actions apply to #12 acme/app unless you pick another.")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Pull requests (3)" })).toBeInTheDocument();
  });

  it("list_should_collapseEarlierMergedPrsBehindAButton_When_OneOpenAndFourMerged", async () => {
    const user = userEvent.setup();
    const prs = [makePr({ number: 12, isActive: true }), merged(11, 1), merged(10, 2), merged(9, 3), merged(8, 4)];
    render(<PullRequestList pullRequests={prs} actions={fakeActions()} nowMs={NOW} />);
    expect(screen.getAllByRole("listitem")).toHaveLength(3);
    const toggle = screen.getByRole("button", { name: "2 earlier PRs" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    await user.click(toggle);
    expect(screen.getAllByRole("listitem")).toHaveLength(5);
    const hide = screen.getByRole("button", { name: "Hide earlier PRs" });
    expect(hide).toHaveAttribute("aria-expanded", "true");
    expect(hide).toHaveFocus();
  });

  it("list_should_markActiveRowCurrentAndGiveEveryLinkAUniqueNameWithTargetAndNewTab", () => {
    render(<PullRequestList pullRequests={three()} actions={fakeActions()} nowMs={NOW} />);
    expect(screen.getByTestId("pr-row-12")).toHaveAttribute("aria-current", "true");
    expect(screen.getByTestId("pr-row-11")).not.toHaveAttribute("aria-current");
    const names = screen.getAllByRole("link").map((l) => l.getAttribute("aria-label"));
    expect(names[0]).toMatch(/#12 acme\/app.*opens in new tab/);
    expect(new Set(names).size).toBe(names.length);
  });

  it("list_should_giveEveryRowControlAUniqueNameContainingTheTarget", () => {
    render(<PullRequestList pullRequests={three()} actions={fakeActions()} nowMs={NOW} />);
    const names = screen.getAllByTestId(/^pr-row-/).flatMap((row) =>
      within(row).getAllByRole("button").map((b) => b.getAttribute("aria-label") ?? ""));
    expect(names.every((n) => /#\d+ acme\/app/.test(n))).toBe(true);
    expect(new Set(names).size).toBe(names.length);
  });

  it("list_should_showOneRowWithNoRoleChipDisclosureOrHint_When_LegacySinglePr", () => {
    render(<PullRequestList pullRequests={[makePr({ number: 7, isActive: true, role: "primary" })]} actions={fakeActions()} nowMs={NOW} />);
    expect(screen.getAllByRole("listitem")).toHaveLength(1);
    expect(screen.queryByText("primary")).not.toBeInTheDocument();
    expect(screen.queryByTestId("pr-disclosure")).not.toBeInTheDocument();
    expect(screen.queryByText(/Actions apply to/)).not.toBeInTheDocument();
  });

  it("list_should_showRoleChipOnlyForFollowupAndStacked", () => {
    render(
      <PullRequestList
        pullRequests={[makePr({ number: 3, role: "stacked" }), makePr({ number: 2, role: "followup" }), makePr({ number: 1, role: "unknown" })]}
        actions={fakeActions()}
        nowMs={NOW}
      />,
    );
    expect(screen.getByText("stacked")).toBeInTheDocument();
    expect(screen.getByText("followup")).toBeInTheDocument();
    expect(screen.queryByText("unknown")).not.toBeInTheDocument();
  });

  it("list_should_showCiAndReviewSummaryOnlyForNonTerminalRows", () => {
    render(
      <PullRequestList
        pullRequests={[makePr({ number: 12, isActive: true, checkConclusion: "success", approvedCount: 1 }), makePr({ number: 11, state: "merged", checkConclusion: "success" })]}
        actions={fakeActions()}
        nowMs={NOW}
      />,
    );
    expect(within(screen.getByTestId("pr-row-12")).getByText("CI passing")).toBeInTheDocument();
    expect(within(screen.getByTestId("pr-row-11")).queryByText("CI passing")).not.toBeInTheDocument();
  });

  it("list_should_capAtFiveRowsWithShowAll_When_Phone", async () => {
    setPhone(true);
    const user = userEvent.setup();
    const prs = [makePr({ number: 20, isActive: true }), ...[1, 2, 3, 4, 5, 6].map((n) => merged(n, n))];
    render(<PullRequestList pullRequests={prs} actions={fakeActions()} nowMs={NOW} />);
    expect(screen.getAllByRole("listitem")).toHaveLength(5);
    await user.click(screen.getByRole("button", { name: "Show all (7)" }));
    expect(screen.getAllByRole("listitem")).toHaveLength(7);
  });

  it("list_should_showCreatePullRequestButton_When_NoPrs", async () => {
    const onCreate = jest.fn();
    render(<PullRequestList pullRequests={[]} actions={fakeActions()} onCreatePullRequest={onCreate} />);
    expect(screen.getByText("No pull requests yet.")).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "Create pull request" }));
    expect(onCreate).toHaveBeenCalled();
  });
});

describe("PullRequestList load states", () => {
  it("list_should_showSkeletonThenErrorWithRetryAndLegacyLink_When_LoadingTakesTenSeconds", () => {
    jest.useFakeTimers();
    try {
      const retry = jest.fn();
      render(<PullRequestList pullRequests={[]} loading actions={fakeActions()} onRetryLoad={retry} fallbackUrl="https://github.com/acme/app/pull/7" />);
      expect(screen.getByTestId("pr-list-loading")).toHaveAttribute("aria-busy", "true");
      expect(screen.getByText("Loading pull requests")).toBeInTheDocument();
      act(() => void jest.advanceTimersByTime(10_001));
      expect(screen.getByRole("alert")).toHaveTextContent("Couldn't load pull requests");
      expect(screen.getByRole("link", { name: "Open the pull request on GitHub" })).toHaveAttribute("href", "https://github.com/acme/app/pull/7");
      fireEvent.click(screen.getByRole("button", { name: "Retry" }));
      expect(retry).toHaveBeenCalled();
    } finally {
      jest.useRealTimers();
    }
  });

  it("list_should_showAlertWithRetry_When_LoadErrorFlagged", () => {
    render(<PullRequestList pullRequests={[]} loadError onRetryLoad={jest.fn()} actions={fakeActions()} />);
    expect(screen.getByRole("alert")).toHaveTextContent("Couldn't load pull requests");
  });

  it("styles_should_wrapLongTextAndKeepTouchTargetsAtFortyFourPixels_ForPhones", () => {
    const css = fs.readFileSync(path.resolve(__dirname, "PullRequestList.css.ts"), "utf-8");
    const dialogs = fs.readFileSync(path.resolve(__dirname, "PullRequestDialogs.css.ts"), "utf-8");
    expect(css).toContain('overflowWrap: "anywhere"');
    expect(css).toContain("minHeight: MIN_TOUCH_TARGET");
    expect(css).toContain("--bottom-nav-height");
    expect(dialogs).toContain("alignItems: \"flex-end\"");
    expect(css).not.toMatch(/overflowX: "(scroll|auto)"/);
  });

  it("skeleton_should_stopAnimatingUnderReducedMotion", () => {
    const css = fs.readFileSync(path.resolve(__dirname, "PullRequestList.css.ts"), "utf-8");
    expect(css).toMatch(/prefers-reduced-motion: reduce[^}]*animation: "none"/s);
  });
});

describe("PullRequestList poll errors, holds and refresh (U-147)", () => {
  it("one_failing_row_should_showCantRefreshChipAndRetryWhileOthersStayActionable", async () => {
    const user = userEvent.setup();
    const actions = fakeActions();
    const prs = [makePr({ number: 12, isActive: true }), makePr({ number: 14, pollError: PollError.AUTH })];
    render(<PullRequestList pullRequests={prs} actions={actions} nowMs={NOW} />);
    expect(screen.getByTestId("pr-error-14")).toHaveTextContent("Can't refresh: GitHub auth failed");
    expect(screen.queryByTestId("pr-error-12")).not.toBeInTheDocument();
    await user.click(screen.getByTestId("pr-menu-14"));
    await user.click(screen.getByRole("menuitem", { name: "Retry" }));
    expect(actions.refresh).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("pr-merge-12")).toBeEnabled();
  });

  it.each([
    [PollError.RATE_LIMITED, /^Rate limited until \d{2}:\d{2}$/],
    [PollError.NOT_FOUND, /^Can't reach this PR on GitHub \(last seen 2d ago\)$/],
    [PollError.REPO_INACCESSIBLE, /^Can't reach this PR on GitHub \(last seen 2d ago\)$/],
    [PollError.NETWORK, /^Can't refresh$/],
  ])("row_should_mapPollError_%s_ToItsOwnCopy", (pollError, expected) => {
    const pr = makePr({ number: 5, pollError });
    pr.rateLimitResetAt = timestampFromMs(NOW + 1000);
    pr.lastPolledAt = timestampFromMs(NOW - 2 * DAY);
    render(<PullRequestList pullRequests={[pr]} actions={fakeActions()} nowMs={NOW} />);
    expect(screen.getByTestId("pr-error-5").textContent).toMatch(expected);
  });

  it("row_should_showCheckingForAnUnpolledRow", () => {
    render(<PullRequestList pullRequests={[makePr({ number: 5, state: "unknown" })]} actions={fakeActions()} nowMs={NOW} />);
    expect(screen.getByTestId("pr-row-5")).toHaveTextContent("Checking...");
  });

  const holdCases: Array<[string, PrHold_Kind, RegExp]> = [
    ["open", PrHold_Kind.OPEN_PR, /^Local files kept: #23 is still open\. They are removed after #23 is merged or closed\.$/],
    ["unverified", PrHold_Kind.UNVERIFIED, /^Local files kept until #23 is confirmed closed or merged\./],
    ["unknown", PrHold_Kind.UNKNOWN, /^Local files kept: GitHub can't be reached for #23/],
    ["superseded", PrHold_Kind.SUPERSEDED_ROUND, /^Kept for #23 from an earlier round; it is still open\.$/],
    ["session active", PrHold_Kind.SESSION_ACTIVE, /^Merged\. The session is still active, so the item isn't done yet\.$/],
    ["max hold", PrHold_Kind.MAX_HOLD, /^The session was still active 1 hour after #23 merged/],
  ];
  it.each(holdCases)("list_should_renderHeldLineWithReasonInPlainWording_When_%sHold", (_n, kind, expected) => {
    render(<PullRequestList pullRequests={[makePr({ number: 23 })]} holds={[makeHold(kind, 23)]} actions={fakeActions()} nowMs={NOW} />);
    const line = screen.getByTestId("pr-hold-23");
    expect(line.textContent).toMatch(expected);
    expect(line.textContent?.toLowerCase()).not.toContain("worktree");
  });

  it("list_should_renderNoHeldLine_When_NothingHolds", () => {
    render(<PullRequestList pullRequests={three()} holds={[]} actions={fakeActions()} nowMs={NOW} />);
    expect(screen.queryByTestId("pr-hold-12")).not.toBeInTheDocument();
  });

  it("list_should_showDistinctCountingAndExhaustedUnverifiedCopyAndRefreshTheRow", async () => {
    const user = userEvent.setup();
    const actions = fakeActions();
    const seeded = (over: { verifyAttempts?: number; verifyExhausted?: boolean } = {}) => [makePr({ number: 9, state: "unknown", outcomeUnverified: true, verifyAttemptLimit: 5, ...over })];
    const { unmount } = render(<PullRequestList pullRequests={seeded({ verifyAttempts: 2 })} actions={actions} nowMs={NOW} />);
    const row = screen.getByTestId("pr-row-9");
    expect(row).toHaveTextContent("Closed or merged (unverified)");
    expect(row).toHaveTextContent("Checking... (attempt 2 of 5)");
    expect(row).not.toHaveTextContent("Couldn't verify");
    expect(row).not.toHaveTextContent(/Merged \d/);
    await user.click(screen.getByTestId("pr-refresh"));
    expect(actions.refresh).toHaveBeenCalledTimes(1);
    await user.click(screen.getByTestId("pr-menu-9"));
    await user.click(screen.getByRole("menuitem", { name: "Refresh" }));
    expect(actions.refresh).toHaveBeenCalledTimes(2);
    unmount();

    render(<PullRequestList pullRequests={seeded({ verifyAttempts: 5, verifyExhausted: true })} actions={actions} nowMs={NOW} />);
    expect(screen.getByTestId("pr-row-9")).toHaveTextContent("Couldn't verify after 5 attempts. Treated as closed.");
    expect(screen.getByTestId("pr-row-9")).not.toHaveTextContent("Checking...");
  });

  it("list_should_showTheServerPrTitleInTheRowAndItsLinkName", () => {
    render(<PullRequestList pullRequests={[makePr({ number: 12, isActive: true, title: "Add token refresh", headBranch: "tok" })]} actions={fakeActions()} nowMs={NOW} />);
    expect(screen.getByTestId("pr-title-12")).toHaveTextContent("Add token refresh");
    expect(screen.getByTestId("pr-link-12")).toHaveAccessibleName("Pull request #12 acme/app, Add token refresh, branch tok, opens in new tab");
  });

  it("askToFix_should_showTheServerOutcomeNoticeInTheNote", async () => {
    const user = userEvent.setup();
    const actions = fakeActions({ askToFix: jest.fn(async () => ({ ok: true as const, notice: "Request sent to refactor-auth (#12 acme/app)." })) });
    render(<PullRequestList pullRequests={[makePr({ number: 12, isActive: true })]} actions={actions} nowMs={NOW} />);
    await user.click(screen.getByRole("button", { name: "Ask to fix #12 acme/app" }));
    expect(actions.askToFix).toHaveBeenCalledWith(expect.objectContaining({ number: 12 }));
    expect(await screen.findByTestId("pr-note")).toHaveTextContent("Request sent to refactor-auth (#12 acme/app).");
  });

  it("track_should_saySoInTheNote_When_TheServerReportsTheSessionAlreadyTracksIt", async () => {
    const user = userEvent.setup();
    const actions = fakeActions({ attachByUrl: jest.fn(async () => ({ ok: true as const, notice: "This session is already tracking #12 acme/app." })) });
    render(<PullRequestList pullRequests={three()} actions={actions} nowMs={NOW} />);
    await user.click(screen.getByTestId("pr-header-menu"));
    await user.click(screen.getByTestId("pr-track-by-url"));
    await user.type(screen.getByLabelText("Pull request URL"), "https://github.com/acme/app/pull/12");
    await user.click(screen.getByRole("button", { name: "Track" }));
    expect(await screen.findByTestId("pr-note")).toHaveTextContent("already tracking #12 acme/app");
  });

  it("header_refresh_should_beBusyWhileRefreshingAndShowFailureInline", async () => {
    const user = userEvent.setup();
    let release: (r: PrActionResult) => void = () => {};
    const actions = fakeActions({ refresh: jest.fn(() => new Promise<PrActionResult>((r) => { release = r; })) });
    render(<PullRequestList pullRequests={three()} actions={actions} nowMs={NOW} />);
    await user.click(screen.getByTestId("pr-refresh"));
    expect(screen.getByTestId("pr-refresh")).toHaveAttribute("aria-busy", "true");
    expect(screen.getByTestId("pr-refresh")).toHaveTextContent("Refreshing");
    await act(async () => release(failure("network", "Couldn't reach the server")));
    expect(screen.getByRole("alert")).toHaveTextContent("Couldn't reach the server");
    expect(screen.getByTestId("pr-refresh")).toHaveTextContent("Refresh");
  });
});

describe("PullRequestList dialogs (S3)", () => {
  it("merge_dialog_should_nameTheActivePrDefaultToMergeAndPassTheRowRef", async () => {
    const user = userEvent.setup();
    const actions = fakeActions();
    render(<PullRequestList pullRequests={[makePr({ number: 12, isActive: true, baseBranch: "main" }), merged(11, 1)]} actions={actions} nowMs={NOW} />);
    await user.click(screen.getByTestId("pr-merge-12"));
    const dialog = screen.getByRole("dialog", { name: "Merge #12 acme/app into main?" });
    expect(within(dialog).getByText("This is the active pull request.")).toBeInTheDocument();
    expect(within(dialog).getByLabelText("Merge method")).toHaveValue("merge");
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));
    expect(actions.merge).toHaveBeenCalledWith({ host: "", owner: "acme", repo: "app", number: 12 }, "merge");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("merge_should_targetTheChosenRowRegardlessOfActive", async () => {
    const user = userEvent.setup();
    const actions = fakeActions();
    render(<PullRequestList pullRequests={[makePr({ number: 12, isActive: true }), makePr({ number: 14 })]} actions={actions} nowMs={NOW} />);
    await user.click(screen.getByTestId("pr-menu-14"));
    await user.click(screen.getByTestId("pr-menu-merge-14"));
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveAccessibleName(/#14 acme\/app/);
    await user.selectOptions(within(dialog).getByLabelText("Merge method"), "rebase");
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));
    expect(actions.merge).toHaveBeenCalledWith(expect.objectContaining({ number: 14 }), "rebase");
  });

  it("dialog_should_stayOpenWithInlineAlertRetryAndCancel_When_MergeFails", async () => {
    const user = userEvent.setup();
    const actions = fakeActions({ merge: jest.fn().mockResolvedValueOnce(failure("other", "checks are failing")).mockResolvedValue(OK) });
    render(<PullRequestList pullRequests={[makePr({ number: 12, isActive: true })]} actions={actions} nowMs={NOW} />);
    await user.click(screen.getByTestId("pr-merge-12"));
    await user.click(screen.getByRole("button", { name: "Merge" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Can't merge #12: checks are failing");
    expect(within(dialog).getByRole("link", { name: "Open on GitHub" })).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Cancel" })).toBeEnabled();
    await user.click(within(dialog).getByRole("button", { name: "Retry merge" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("dialog_should_switchToDismissOnlyAndRefresh_When_PrWasAlreadyMerged", async () => {
    const user = userEvent.setup();
    const actions = fakeActions({ merge: jest.fn().mockResolvedValue(failure("already_merged", "already merged")) });
    render(<PullRequestList pullRequests={[makePr({ number: 12, isActive: true })]} actions={actions} nowMs={NOW} />);
    await user.click(screen.getByTestId("pr-merge-12"));
    await user.click(screen.getByRole("button", { name: "Merge" }));
    expect(screen.getByRole("alert")).toHaveTextContent("#12 was already merged");
    expect(screen.queryByRole("button", { name: /Merge$/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Dismiss" })).toBeInTheDocument();
    expect(actions.refresh).toHaveBeenCalled();
  });

  it("dialog_should_trapFocusCloseOnEscAndReturnFocusToTheInvokingControl", async () => {
    const user = userEvent.setup();
    render(<PullRequestList pullRequests={[makePr({ number: 12, isActive: true })]} actions={fakeActions()} nowMs={NOW} />);
    const trigger = screen.getByTestId("pr-merge-12");
    await user.click(trigger);
    const dialog = screen.getByRole("dialog");
    expect(dialog).toContainElement(document.activeElement as HTMLElement);
    for (let i = 0; i < 6; i += 1) {
      await user.tab();
      expect(dialog).toContainElement(document.activeElement as HTMLElement);
    }
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it("menuInvokedDialog_should_returnFocusToTheRowMenuAndTrackToTheHeaderMenu_When_CancelledOrEscaped", async () => {
    const user = userEvent.setup();
    render(<PullRequestList pullRequests={[makePr({ number: 12, isActive: true })]} actions={fakeActions()} nowMs={NOW} />);
    await user.click(screen.getByTestId("pr-menu-12"));
    await user.click(screen.getByTestId("pr-menu-close-12"));
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.getByTestId("pr-menu-12")).toHaveFocus());

    await user.click(screen.getByTestId("pr-header-menu"));
    await user.click(screen.getByTestId("pr-track-by-url"));
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.getByTestId("pr-header-menu")).toHaveFocus());
  });

  it("close_and_detach_dialogs_should_stateTheGitHubEffect", async () => {
    const user = userEvent.setup();
    render(<PullRequestList pullRequests={[makePr({ number: 12, isActive: true })]} actions={fakeActions()} nowMs={NOW} />);
    await user.click(screen.getByTestId("pr-menu-12"));
    await user.click(screen.getByTestId("pr-menu-close-12"));
    expect(screen.getByRole("dialog", { name: "Close #12 acme/app without merging?" })).toHaveTextContent("closed on GitHub, not merged");
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await user.click(screen.getByTestId("pr-menu-12"));
    await user.click(screen.getByTestId("pr-menu-detach-12"));
    const detach = screen.getByRole("dialog", { name: "Remove #12 acme/app from this session?" });
    expect(detach).toHaveTextContent("The PR stays on GitHub.");
    expect(detach).toHaveTextContent("won't be added back automatically");
    expect(detach).toHaveTextContent("Track a PR by URL");
  });

  it("copyLink_should_copyTheDerivedUrl_When_ListFormPrHasNoUrl", async () => {
    const user = userEvent.setup();
    const original = navigator.clipboard;
    const writeText = jest.fn().mockResolvedValue(undefined);
    try {
      setClipboard({ writeText });
      render(<PullRequestList pullRequests={[makePr({ number: 12, url: "", host: "ghe.example.net" })]} actions={fakeActions()} nowMs={NOW} />);
      await user.click(screen.getByTestId("pr-menu-12"));
      await user.click(screen.getByTestId("pr-menu-copyLink-12"));
      await screen.findByTestId("pr-note");
      expect(writeText).toHaveBeenCalledWith("https://ghe.example.net/acme/app/pull/12");
    } finally {
      setClipboard(original);
    }
  });

  it("detach_undo_should_reattachTheDerivedUrl_When_ListFormPrHasNoUrl", async () => {
    const user = userEvent.setup();
    const actions = fakeActions();
    render(<Harness initial={[makePr({ number: 12, isActive: true }), makePr({ number: 11, url: "" })]} actions={actions} />);
    await user.click(screen.getByTestId("pr-menu-11"));
    await user.click(screen.getByTestId("pr-menu-detach-11"));
    await user.click(screen.getByRole("button", { name: "Not mine" }));
    await user.click(within(await screen.findByTestId("pr-undo-toast")).getByRole("button", { name: "Undo" }));
    expect(actions.attachByUrl).toHaveBeenCalledWith("https://github.com/acme/app/pull/11");
  });

  it("detach_should_showUndoToastThatReattachesAndOfferTrackByUrl", async () => {
    const user = userEvent.setup();
    const actions = fakeActions();
    render(<Harness initial={three()} actions={actions} />);
    await user.click(screen.getByTestId("pr-menu-11"));
    await user.click(screen.getByTestId("pr-menu-detach-11"));
    await user.click(screen.getByRole("button", { name: "Not mine" }));
    expect(actions.detach).toHaveBeenCalledWith(expect.objectContaining({ number: 11 }));
    const toast = await screen.findByTestId("pr-undo-toast");
    expect(toast).toHaveTextContent("Stopped tracking #11.");
    expect(toast).toHaveTextContent("Undo, or use Track a PR by URL.");
    await user.click(within(toast).getByRole("button", { name: "Undo" }));
    expect(actions.attachByUrl).toHaveBeenCalledWith("https://github.com/acme/app/pull/11");
    expect(screen.queryByTestId("pr-undo-toast")).not.toBeInTheDocument();
  });

  it("track_dialog_should_showInlineErrorsAndCloseOnSuccess", async () => {
    const user = userEvent.setup();
    const attach = jest.fn().mockResolvedValueOnce(failure("invalid_url", "x")).mockResolvedValueOnce(failure("not_found", "x")).mockResolvedValueOnce(failure("other", "Already tracked")).mockResolvedValue(OK);
    render(<PullRequestList pullRequests={three()} actions={fakeActions({ attachByUrl: attach })} nowMs={NOW} />);
    await user.click(screen.getByTestId("pr-header-menu"));
    await user.click(screen.getByRole("menuitem", { name: "Track a PR by URL" }));
    const dialog = screen.getByRole("dialog", { name: "Track a pull request by URL" });
    await user.type(within(dialog).getByLabelText("Pull request URL"), "https://github.com/acme/app/pull/99");
    const submit = within(dialog).getByRole("button", { name: "Track" });
    await user.click(submit);
    expect(within(dialog).getByRole("alert")).toHaveTextContent("not a GitHub pull request URL");
    await user.click(submit);
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Not found on GitHub.");
    await user.click(submit);
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Already tracked");
    await user.click(submit);
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(attach).toHaveBeenLastCalledWith("https://github.com/acme/app/pull/99");
  });
});

describe("PullRequestList pin toast and focus (UX-S3-3, UX-S3-6)", () => {
  afterEach(() => jest.useRealTimers());

  it("pin_toast_should_offerUndoThatRestoresPreviousPinAndDismissAfterEightSeconds", async () => {
    jest.useFakeTimers();
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime });
    const actions = fakeActions();
    render(<PullRequestList pullRequests={[makePr({ number: 12, isActive: true, pinned: true }), makePr({ number: 14 })]} actions={actions} nowMs={NOW} />);
    await user.click(screen.getByTestId("pr-menu-14"));
    await user.click(screen.getByTestId("pr-menu-pin-14"));
    const toast = await screen.findByTestId("pr-undo-toast");
    expect(toast).toHaveTextContent("Pinned #14 as current.");
    expect(actions.pin).toHaveBeenCalledWith(expect.objectContaining({ number: 14 }));
    await user.click(within(toast).getByRole("button", { name: "Undo" }));
    expect(actions.pin).toHaveBeenLastCalledWith(expect.objectContaining({ number: 12 }));

    await user.click(screen.getByTestId("pr-menu-14"));
    await user.click(screen.getByTestId("pr-menu-pin-14"));
    await screen.findByTestId("pr-undo-toast");
    act(() => void jest.advanceTimersByTime(7_900));
    expect(screen.getByTestId("pr-undo-toast")).toBeInTheDocument();
    act(() => void jest.advanceTimersByTime(200));
    expect(screen.queryByTestId("pr-undo-toast")).not.toBeInTheDocument();
  });

  it("pin_toast_should_pauseWhileFocusedOrHoveredAndKeepUndoReachable", async () => {
    jest.useFakeTimers();
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime });
    render(<PullRequestList pullRequests={[makePr({ number: 12, isActive: true }), makePr({ number: 14 })]} actions={fakeActions()} nowMs={NOW} />);
    await user.click(screen.getByTestId("pr-menu-14"));
    await user.click(screen.getByTestId("pr-menu-pin-14"));
    const toast = await screen.findByTestId("pr-undo-toast");
    act(() => within(toast).getByRole("button", { name: "Undo" }).focus());
    act(() => void jest.advanceTimersByTime(30_000));
    expect(screen.getByTestId("pr-undo-toast")).toBeInTheDocument();
    act(() => (document.activeElement as HTMLElement).blur());
    act(() => void jest.advanceTimersByTime(8_100));
    expect(screen.queryByTestId("pr-undo-toast")).not.toBeInTheDocument();
  });

  it("keyboard_pin_should_moveFocusToUndoAndEscReturnsFocusToTheInvokingControl", async () => {
    const user = userEvent.setup();
    render(<PullRequestList pullRequests={[makePr({ number: 12, isActive: true }), makePr({ number: 14 })]} actions={fakeActions()} nowMs={NOW} />);
    screen.getByTestId("pr-menu-14").focus();
    await user.keyboard("{Enter}");
    screen.getByTestId("pr-menu-pin-14").focus();
    await user.keyboard("{Enter}");
    const undo = await screen.findByRole("button", { name: "Undo" });
    await waitFor(() => expect(undo).toHaveFocus());
    await user.keyboard("{Escape}");
    expect(screen.queryByTestId("pr-undo-toast")).not.toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId("pr-menu-14")).toHaveFocus());
  });

  it("pointer_pin_should_notStealFocusToTheToast", async () => {
    const user = userEvent.setup();
    render(<PullRequestList pullRequests={[makePr({ number: 12, isActive: true }), makePr({ number: 14 })]} actions={fakeActions()} nowMs={NOW} />);
    await user.click(screen.getByTestId("pr-menu-14"));
    await user.click(screen.getByTestId("pr-menu-pin-14"));
    const undo = await screen.findByRole("button", { name: "Undo" });
    expect(undo).not.toHaveFocus();
  });

  it("pin_failure_should_alertWithTheTargetAndShowNoToast", async () => {
    const user = userEvent.setup();
    render(<PullRequestList pullRequests={[makePr({ number: 12, isActive: true }), makePr({ number: 14 })]} actions={fakeActions({ pin: jest.fn().mockResolvedValue(failure("other", "boom")) })} nowMs={NOW} />);
    await user.click(screen.getByTestId("pr-menu-14"));
    await user.click(screen.getByTestId("pr-menu-pin-14"));
    expect(await screen.findByRole("alert")).toHaveTextContent("Couldn't pin #14: boom");
    expect(screen.queryByTestId("pr-undo-toast")).not.toBeInTheDocument();
  });

  async function detachRow(user: ReturnType<typeof userEvent.setup>, n: number) {
    await user.click(screen.getByTestId(`pr-menu-${n}`));
    await user.click(screen.getByTestId(`pr-menu-detach-${n}`));
    await user.click(screen.getByRole("button", { name: "Not mine" }));
    await screen.findByTestId("pr-undo-toast");
  }

  it("detach_should_moveFocusToTheNextRow_When_AMiddleRowIsRemoved", async () => {
    const user = userEvent.setup();
    render(<Harness initial={three()} actions={fakeActions()} />);
    await detachRow(user, 11);
    await waitFor(() => expect(screen.getByTestId("pr-link-10")).toHaveFocus());
  });

  it("detach_should_moveFocusToThePreviousRow_When_TheLastRowIsRemoved", async () => {
    const user = userEvent.setup();
    render(<Harness initial={three()} actions={fakeActions()} />);
    await detachRow(user, 10);
    await waitFor(() => expect(screen.getByTestId("pr-link-11")).toHaveFocus());
  });

  it("detach_should_moveFocusToTheHeading_When_TheOnlyRowIsRemoved", async () => {
    const user = userEvent.setup();
    render(<Harness initial={[makePr({ number: 12, isActive: true })]} actions={fakeActions()} />);
    await detachRow(user, 12);
    await waitFor(() => expect(screen.getByRole("heading", { name: "Pull requests" })).toHaveFocus());
    expect(document.body).not.toHaveFocus();
  });
});

describe("PullRequestList live announcements (UX-S10-3, A-10)", () => {
  afterEach(() => jest.useRealTimers());

  it("status_should_announceOnceAfterTwoSecondsAndNeverOnInitialRender", () => {
    jest.useFakeTimers();
    const actions = fakeActions();
    const open = [makePr({ number: 12, isActive: true }), makePr({ number: 14 })];
    const { rerender } = render(<PullRequestList pullRequests={open} actions={actions} nowMs={NOW} />);
    const region = screen.getByTestId("pr-live-region");
    act(() => void jest.advanceTimersByTime(5_000));
    expect(region).toHaveTextContent("");

    rerender(<PullRequestList pullRequests={[{ ...open[0], state: "merged" } as SessionPullRequest, open[1]]} actions={actions} nowMs={NOW} />);
    act(() => void jest.advanceTimersByTime(1_000));
    rerender(<PullRequestList pullRequests={[{ ...open[0], state: "merged" } as SessionPullRequest, { ...open[1], state: "closed" } as SessionPullRequest]} actions={actions} nowMs={NOW} />);
    expect(region).toHaveTextContent("");
    act(() => void jest.advanceTimersByTime(1_100));
    expect(region.textContent).toBe("PR #12 is now merged. PR #14 is now closed");
  });

  it("status_should_announceANewHoldOnceAndAgainOnlyAfterItDisappears", () => {
    jest.useFakeTimers();
    const actions = fakeActions();
    const prs = [makePr({ number: 23 })];
    const hold = makeHold(PrHold_Kind.OPEN_PR, 23);
    const { rerender } = render(<PullRequestList pullRequests={prs} holds={[]} actions={actions} nowMs={NOW} />);
    const region = screen.getByTestId("pr-live-region");
    rerender(<PullRequestList pullRequests={prs} holds={[hold]} actions={actions} nowMs={NOW} />);
    act(() => void jest.advanceTimersByTime(2_100));
    expect(region.textContent).toContain("Local files kept: #23 is still open");

    rerender(<PullRequestList pullRequests={[...prs]} holds={[makeHold(PrHold_Kind.OPEN_PR, 23)]} actions={actions} nowMs={NOW} />);
    act(() => void jest.advanceTimersByTime(2_100));
    expect(region.textContent).toContain("Local files kept: #23 is still open");
    const before = region.textContent;
    rerender(<PullRequestList pullRequests={prs} holds={[]} actions={actions} nowMs={NOW} />);
    rerender(<PullRequestList pullRequests={prs} holds={[hold]} actions={actions} nowMs={NOW} />);
    act(() => void jest.advanceTimersByTime(2_100));
    expect(region.textContent).toBe(before);
  });

  it("headerMenu_should_returnFocusToTriggerOnEscape_AndCloseOnOutsideMousedown", async () => {
    const user = userEvent.setup();
    render(<div><PullRequestList pullRequests={three()} actions={fakeActions()} nowMs={NOW} /><button>outside</button></div>);
    const trigger = screen.getByTestId("pr-header-menu");
    await user.click(trigger);
    expect(screen.getByTestId("pr-track-by-url")).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(screen.queryByTestId("pr-track-by-url")).toBeNull();
    expect(trigger).toHaveFocus();

    await user.click(trigger);
    expect(screen.getByTestId("pr-track-by-url")).toBeInTheDocument();
    fireEvent.mouseDown(screen.getByRole("button", { name: "outside" }));
    expect(screen.queryByTestId("pr-track-by-url")).toBeNull();
  });

  it("copyLink_should_showAlertNote_When_ClipboardMissingOrDenied", async () => {
    const user = userEvent.setup();
    const original = navigator.clipboard;
    try {
      for (const clipboard of [undefined, { writeText: jest.fn().mockRejectedValue(new Error("denied")) }]) {
        setClipboard(clipboard);
        const { unmount } = render(<PullRequestList pullRequests={[makePr({ number: 12 })]} actions={fakeActions()} nowMs={NOW} />);
        await user.click(screen.getByTestId("pr-menu-12"));
        await user.click(screen.getByTestId("pr-menu-copyLink-12"));
        const note = await screen.findByTestId("pr-note");
        expect(note).toHaveTextContent("Couldn't copy link");
        expect(note).toHaveAttribute("role", "alert");
        unmount();
      }
    } finally {
      setClipboard(original);
    }
  });
});
