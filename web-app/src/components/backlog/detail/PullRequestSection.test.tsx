import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FeatureFlag } from "@/gen/session/v1/session_pb";
import { PrHold_Kind } from "@/gen/session/v1/types_pb";
import { failure, OK, type PrTarget } from "@/lib/pr/actionResult";
import type { ItemPullRequestActions } from "@/lib/hooks/useItemPullRequestActions";
import { makeHold, makePr } from "@/lib/pr/__testUtils__/pullRequests";
import { PullRequestSection, prStatusLabel } from "./PullRequestSection";
import { StuckReason } from "@/gen/session/v1/backlog_pb";
import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";

let mockFlags: FeatureFlag[] = [];
jest.mock("@/lib/features/flagStore", () => ({
  loadFlags: () => Promise.resolve(mockFlags),
  peekFlags: () => mockFlags,
  subscribeFlags: () => () => {},
}));
const backlogFlags = (on: boolean) =>
  [{ name: "session_multi_pr", enabled: false, scopes: ["reader:poller", "reader:backlog"].map((scope) => ({ scope, enabled: on })) }] as unknown as FeatureFlag[];

function makeSession(overrides: Partial<LinkedSession> = {}): LinkedSession {
  return {
    entityId: `entity-${Math.random()}`,
    sessionId: `session-${Math.random()}`,
    role: "work",
    estimatedCostUsd: 0,
    ...overrides,
  };
}

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "itm_pr_section",
    title: "Item with a PR",
    status: "pr_pending",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    autoApprovePlan: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    notes: "",
    statusEvents: [],
    progressNotes: [],
    activityNotes: [],
    totalEstimatedCostUsd: 0,
    prUrl: "https://github.com/tstapler/stapler-squad/pull/700",
    prNumber: 700,
    ...overrides,
  };
}

describe("PullRequestSection", () => {
  it("renders the unmodified GitHubBadge plus an adjacent Jules marker when the PR-producing session role is jules_work", () => {
    const item = makeItem({
      linkedSessions: [makeSession({ sessionId: "jules-sessions/xyz", role: "jules_work" })],
    });
    render(<PullRequestSection item={item} actionLoading={null} onMarkDone={jest.fn()} />);

    const prLink = screen.getByRole("link", { name: /View GitHub Pull Request #700/ });
    expect(prLink).toHaveAttribute("href", "https://github.com/tstapler/stapler-squad/pull/700");

    const marker = screen.getByLabelText("Opened by Jules");
    expect(marker).toHaveTextContent("Jules");
  });

  it("renders no Jules marker when the PR-producing session's role is not jules_work", () => {
    const item = makeItem({
      linkedSessions: [makeSession({ sessionId: "work-1", role: "work" })],
    });
    render(<PullRequestSection item={item} actionLoading={null} onMarkDone={jest.fn()} />);

    expect(screen.getByRole("link", { name: /View GitHub Pull Request #700/ })).toBeInTheDocument();
    expect(screen.queryByLabelText("Opened by Jules")).not.toBeInTheDocument();
  });

  it("still shows Opened by Jules when a review session links after the jules_work session that produced the PR", () => {
    // Regression test: a review-gate session links *after* the jules_work
    // session it reviews (see julesDispatchGate.ts's skipReviewGate flow),
    // so the most-recently-linked session is "review", not "jules_work".
    // The badge must attribute the PR to the session that actually produced
    // it, not simply the last-linked session.
    const item = makeItem({
      linkedSessions: [
        makeSession({ sessionId: "jules-sessions/xyz", role: "jules_work" }),
        makeSession({ sessionId: "review-session-abc", role: "review" }),
      ],
    });
    render(<PullRequestSection item={item} actionLoading={null} onMarkDone={jest.fn()} />);

    expect(screen.getByRole("link", { name: /View GitHub Pull Request #700/ })).toBeInTheDocument();
    const marker = screen.getByLabelText("Opened by Jules");
    expect(marker).toHaveTextContent("Jules");
  });
});

describe("prStatusLabel", () => {
  it("reports needs-fix when PR_NEEDS_FIX is open, even alongside other reasons", () => {
    expect(prStatusLabel([StuckReason.MULTIPLE_REASONS, StuckReason.PR_NEEDS_FIX])).toMatch(/needs fixes/);
  });

  it("reports ready to merge for PR_READY_UNMERGED", () => {
    expect(prStatusLabel([StuckReason.PR_READY_UNMERGED])).toBe("ready to merge");
  });

  it("falls back to waiting for merge with no relevant reason", () => {
    expect(prStatusLabel()).toBe("waiting for merge");
    expect(prStatusLabel([StuckReason.BOUNCING])).toBe("waiting for merge");
  });

  it("renders the derived label in the section", () => {
    render(
      <PullRequestSection
        item={makeItem()}
        actionLoading={null}
        onMarkDone={jest.fn()}
        stuckReasons={[StuckReason.PR_NEEDS_FIX]}
      />
    );
    expect(screen.getByText(/PR #700 — needs fixes/)).toBeInTheDocument();
  });
});

describe("PullRequestSection with the multi-PR list (ux.md S5)", () => {
  const fakeActions = (): jest.Mocked<ItemPullRequestActions> => ({
    stopTracking: jest.fn(async (_pr: PrTarget) => OK),
    trackByUrl: jest.fn(async (_url: string) => OK),
  });
  const renderSection = (item: BacklogItem, prActions?: ItemPullRequestActions) =>
    render(<PullRequestSection item={item} actionLoading={null} onMarkDone={jest.fn()} prActions={prActions} />);
  const threePrs = () => [makePr({ number: 21, state: "merged" }), makePr({ number: 22, state: "merged" }), makePr({ number: 23, itemDirect: true })];

  beforeEach(() => { mockFlags = backlogFlags(true); });
  afterEach(() => { mockFlags = []; });

  it("PullRequestSection should show k of N merged with three rows and a progressbar", async () => {
    renderSection(makeItem({ status: "in_progress", pullRequests: threePrs() }));
    expect(await screen.findByTestId("item-pr-summary")).toHaveTextContent("2 of 3 merged");
    expect(screen.getByTestId("item-pr-row-21")).toBeInTheDocument();
    expect(screen.getByTestId("item-pr-row-22")).toBeInTheDocument();
    expect(screen.getByTestId("item-pr-row-23")).toBeInTheDocument();
    const bar = screen.getByRole("progressbar", { name: "Pull requests merged" });
    expect(bar).toHaveAttribute("aria-valuetext", "2 of 3 pull requests merged");
    expect(bar).toHaveAttribute("value", "2");
    expect(bar).toHaveAttribute("max", "3");
  });

  it("section should name the blocking PR in a sentence", async () => {
    renderSection(makeItem({ pullRequests: threePrs() }));
    expect(await screen.findByTestId("item-pr-rule")).toHaveTextContent("Done when at least one PR is merged and none are open. Waiting on #23 (open).");
  });

  it("section should explain cannot complete and offer Attach another PR when only closed", async () => {
    renderSection(makeItem({ status: "in_progress", pullRequests: [1, 2, 3].map((n) => makePr({ number: n, state: "closed" })) }), fakeActions());
    expect(await screen.findByTestId("item-pr-summary")).toHaveTextContent("0 merged, 3 closed. No merged PR, so the item can't complete.");
    expect(screen.getByRole("button", { name: "Track a PR by URL" })).toBeInTheDocument();
  });

  it("section should render for an archived item and Stop tracking should call the exit with the row's ref", async () => {
    const actions = fakeActions();
    renderSection(makeItem({ status: "archived", pullRequests: [makePr({ number: 31, state: "unknown", host: "github.com", itemDirect: true })] }), actions);
    await userEvent.click(await screen.findByRole("button", { name: "Stop tracking #31 acme/app" }));
    const dialog = screen.getByTestId("item-pr-stop-dialog");
    expect(dialog).toHaveTextContent("It won't be added back automatically");
    await userEvent.click(within(dialog).getByRole("button", { name: "Stop tracking" }));
    await waitFor(() => expect(actions.stopTracking).toHaveBeenCalledWith({ host: "github.com", owner: "acme", repo: "app", number: 31 }));
    expect(await screen.findByText("Stopped tracking #31.")).toBeInTheDocument();
  });

  it("section should offer no Stop tracking on a merged row and none when read-only", async () => {
    const actions = fakeActions();
    const { rerender } = renderSection(makeItem({ pullRequests: threePrs() }), actions);
    await screen.findByTestId("item-pr-row-21");
    expect(screen.queryByTestId("item-pr-stop-21")).toBeNull();
    expect(screen.getByTestId("item-pr-stop-23")).toBeInTheDocument();
    rerender(<PullRequestSection item={makeItem({ pullRequests: threePrs() })} actionLoading={null} onMarkDone={jest.fn()} readOnly prActions={actions} />);
    expect(screen.queryByTestId("item-pr-stop-23")).toBeNull();
    expect(screen.queryByRole("button", { name: "Track a PR by URL" })).toBeNull();
  });

  it("section should offer Stop tracking only for item-direct rows and name the session as the source of the rest", async () => {
    const actions = fakeActions();
    const prs = [makePr({ number: 23, state: "open", itemDirect: true, title: "Wire the UI" }), makePr({ number: 24, state: "open", itemDirect: false })];
    renderSection(makeItem({ pullRequests: prs }), actions);
    expect(await screen.findByTestId("item-pr-stop-23")).toBeInTheDocument();
    expect(screen.queryByTestId("item-pr-stop-24")).toBeNull();
    expect(screen.getByTestId("item-pr-via-session-24")).toHaveTextContent("Tracked by the item's session");
    expect(screen.queryByTestId("item-pr-via-session-23")).toBeNull();
    expect(screen.getByTestId("item-pr-title-23")).toHaveTextContent("Wire the UI");
    expect(screen.getByRole("link", { name: "Pull request #23 acme/app, Wire the UI, opens in new tab" })).toBeInTheDocument();
  });

  it("section should show the server's verify counters on an unverified row", async () => {
    renderSection(makeItem({ pullRequests: [makePr({ number: 25, state: "unknown", outcomeUnverified: true, verifyAttempts: 3, verifyAttemptLimit: 7 })] }));
    expect(await screen.findByTestId("item-pr-row-25")).toHaveTextContent("Checking... (attempt 3 of 7)");
  });

  it("Track a PR by URL should show the inline error and keep the dialog open", async () => {
    const actions = fakeActions();
    actions.trackByUrl.mockResolvedValueOnce(failure("invalid_url", "That is not a GitHub pull request URL."));
    renderSection(makeItem({ pullRequests: threePrs() }), actions);
    await userEvent.click(await screen.findByRole("button", { name: "Track a PR by URL" }));
    await userEvent.type(screen.getByLabelText("Pull request URL"), "https://example.com/nope");
    await userEvent.click(screen.getByRole("button", { name: "Track" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("That is not a GitHub pull request URL.");
    expect(actions.trackByUrl).toHaveBeenCalledWith("https://example.com/nope");
  });

  it("PullRequestSection should show waiting-for-session-idle line with server idle window and the max-hold warning", async () => {
    renderSection(makeItem({
      status: "in_progress",
      pullRequests: [makePr({ number: 21, state: "merged" })],
      prHolds: [makeHold(PrHold_Kind.SESSION_ACTIVE, 21, { idleWindowSeconds: 600 }), makeHold(PrHold_Kind.MAX_HOLD, 21)],
    }));
    const holds = await screen.findAllByTestId("item-pr-hold");
    expect(holds[0]).toHaveTextContent("#21 merged. Waiting for the session to go idle");
    expect(holds[0]).toHaveTextContent("10 minutes idle");
    expect(holds[1]).toHaveTextContent("still active 1 hour after #21 merged");
    for (const h of holds) expect(h.textContent).not.toMatch(/stuck|error|worktree/i);
    expect(screen.getByTestId("item-pr-rule").compareDocumentPosition(holds[0]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("PullRequestSection should render nothing for a non-pr_pending item when the flag is off", async () => {
    mockFlags = backlogFlags(false);
    const { container } = renderSection(makeItem({ status: "done", pullRequests: threePrs() }));
    expect(container).toBeEmptyDOMElement();
  });

  it("PullRequestSection should keep the legacy single block for pr_pending when the flag is off", () => {
    mockFlags = backlogFlags(false);
    renderSection(makeItem({ pullRequests: threePrs() }));
    expect(screen.getByText(/PR #700/)).toBeInTheDocument();
    expect(screen.queryByTestId("item-pr-list")).toBeNull();
  });
});
