import { render, screen } from "@testing-library/react";
import type { FeatureFlag } from "@/gen/session/v1/session_pb";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { makePr } from "@/lib/pr/__testUtils__/pullRequests";
import { LifecycleSummary } from "./LifecycleSummary";
import { VersionControlSection } from "./VersionControlSection";

let mockFlags: FeatureFlag[] = [];
jest.mock("@/lib/features/flagStore", () => ({
  loadFlags: () => Promise.resolve(mockFlags),
  peekFlags: () => mockFlags,
  subscribeFlags: () => () => {},
}));
const backlogFlags = (on: boolean) =>
  [{ name: "session_multi_pr", enabled: false, scopes: ["reader:poller", "reader:backlog"].map((scope) => ({ scope, enabled: on })) }] as unknown as FeatureFlag[];

const item = (over: Partial<BacklogItem> = {}) =>
  ({
    id: "i1", title: "Item", status: "in_progress", priority: 3, acCriteria: [], linkedSessions: [], notes: "", statusEvents: [], progressNotes: [], activityNotes: [],
    totalEstimatedCostUsd: 0, prUrl: "https://github.com/acme/app/pull/23", prNumber: 23,
    pullRequests: [makePr({ number: 21, state: "merged" }), makePr({ number: 22, state: "merged" }), makePr({ number: 23 })],
    ...over,
  }) as unknown as BacklogItem;

afterEach(() => { mockFlags = []; });

describe("item PR list on the other backlog surfaces", () => {
  it("LifecycleSummary should show k of N merged in place of the single badge when the list is on", async () => {
    mockFlags = backlogFlags(true);
    render(<LifecycleSummary item={item()} />);
    expect(await screen.findByTestId("lifecycle-pr-progress")).toHaveTextContent("PRs: 2 of 3 merged");
    expect(screen.queryByTestId("lifecycle-pr-badge")).toBeNull();
  });

  it("LifecycleSummary should keep the single badge when the flag is off", () => {
    render(<LifecycleSummary item={item()} />);
    expect(screen.getByTestId("lifecycle-pr-badge")).toBeInTheDocument();
    expect(screen.queryByTestId("lifecycle-pr-progress")).toBeNull();
  });

  it("VersionControlSection should drop its own PR link when PullRequestSection owns the list", () => {
    mockFlags = backlogFlags(true);
    const { rerender } = render(<VersionControlSection item={item()} widgetData={null} worktreePath={undefined} activeSessionCount={0} defaultExpanded onViewDiff={jest.fn()} onBrowseFiles={jest.fn()} />);
    expect(screen.queryByTestId("vcs-section-pr-link")).toBeNull();
    mockFlags = [];
    rerender(<VersionControlSection item={item({ pullRequests: [] })} widgetData={null} worktreePath={undefined} activeSessionCount={0} defaultExpanded onViewDiff={jest.fn()} onBrowseFiles={jest.fn()} />);
    expect(screen.getByTestId("vcs-section-pr-link")).toBeInTheDocument();
  });
});
