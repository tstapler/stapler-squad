import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SessionCard } from "../SessionCard";
import { SessionRow } from "../SessionRow";
import type { Session } from "@/gen/session/v1/types_pb";
import type { FeatureFlag } from "@/gen/session/v1/session_pb";
import { makePr } from "@/lib/pr/__testUtils__/pullRequests";

jest.mock("@connectrpc/connect", () => require("./sessionCardTestFixtures").mockConnect());
jest.mock("@connectrpc/connect-web", () => require("./sessionCardTestFixtures").mockConnectWeb());
jest.mock("@/lib/contexts/ReviewQueueContext", () => require("./sessionCardTestFixtures").mockReviewQueueContext());
jest.mock("@/lib/contexts/SessionServiceContext", () => require("./sessionCardTestFixtures").mockSessionServiceContext());
jest.mock("@/lib/store", () => require("./sessionCardTestFixtures").mockStore());
let mockFlags: FeatureFlag[] = [];
jest.mock("@/lib/features/flagStore", () => ({
  loadFlags: () => Promise.resolve(mockFlags),
  peekFlags: () => mockFlags,
  subscribeFlags: () => () => {},
}));
jest.mock("@/lib/store/sessionsSlice", () => require("./sessionCardTestFixtures").mockSessionsSlice());
jest.mock("@/lib/hooks/useTerminalSnapshot", () => require("./sessionCardTestFixtures").mockUseTerminalSnapshot());
jest.mock("@/lib/hooks/useFocusTrap", () => require("./sessionCardTestFixtures").mockUseFocusTrap());
jest.mock("@/components/ui/AppLink", () => require("./sessionCardTestFixtures").mockAppLink());
jest.mock("@/components/ui/Modal", () => require("./sessionCardTestFixtures").mockModal());
jest.mock("@/lib/hooks/useSessionActions", () => require("./sessionCardTestFixtures").mockUseSessionActions());

function flags(readerUi: boolean): FeatureFlag[] {
  const scope = (name: string, enabled: boolean) => ({ scope: name, enabled });
  return [{
    name: "session_multi_pr", enabled: false, description: "", statusDetail: "",
    scopes: [scope("reader:poller", true), scope("reader:backlog", true), scope("reader:actions", true), scope("reader:ui", readerUi)],
  }] as unknown as FeatureFlag[];
}

function makeSession(over: Partial<Session> = {}): Session {
  return {
    id: "s1", title: "refactor-auth", status: 1, tags: [], category: "", existingDir: "/tmp/s", branch: "", program: "claude",
    githubOwner: "acme", githubRepo: "app", githubPrNumber: 7, githubPrUrl: "https://github.com/acme/app/pull/7",
    githubPrState: "open", pullRequests: [], prHolds: [],
    ...over,
  } as unknown as Session;
}

const threePrs = () => [
  makePr({ number: 12, isActive: true }),
  makePr({ number: 11, state: "merged" }),
  makePr({ number: 10, state: "merged" }),
];

const surfaces: Array<[string, (s: Session, onClick?: () => void, onOpenInfo?: () => void) => React.ReactElement]> = [
  ["SessionCard", (s, onClick, onOpenInfo) => <SessionCard session={s} onClick={onClick} onOpenInfo={onOpenInfo} />],
  ["SessionRow", (s, onClick, onOpenInfo) => <SessionRow session={s} onClick={onClick} onOpenInfo={onOpenInfo} />],
];

describe.each(surfaces)("%s pull request badge", (_name, renderSurface) => {
  beforeEach(() => { window.location.hash = ""; });

  it("surface_should_showExactlyOneBadgeAndAPlusTwoButton_When_ThreePrsAndFlagOn", async () => {
    mockFlags = flags(true);
    await act(async () => { render(renderSurface(makeSession({ pullRequests: threePrs() }))); });
    await waitFor(() => expect(screen.getByTestId("pr-more-button")).toHaveTextContent("+2"));
    expect(screen.getAllByTestId("pr-chip")).toHaveLength(1);
    expect(screen.getByTestId("pr-chip")).toHaveAccessibleName(/#12, acme\/app, status: open/);
  });

  it("plusTwo_should_openTheSessionOnceAndPointTheUrlAtThePullRequestsSection", async () => {
    mockFlags = flags(true);
    const onClick = jest.fn();
    await act(async () => { render(renderSurface(makeSession({ pullRequests: threePrs() }), onClick)); });
    const more = await screen.findByTestId("pr-more-button");
    await userEvent.setup().click(more);
    expect(onClick).toHaveBeenCalledTimes(1);
    expect(window.location.hash).toBe("#pull-requests");
  });

  it("plusTwo_should_openTheInfoTab_When_TheHostProvidesOnOpenInfo", async () => {
    mockFlags = flags(true);
    const onClick = jest.fn();
    const onOpenInfo = jest.fn();
    await act(async () => { render(renderSurface(makeSession({ pullRequests: threePrs() }), onClick, onOpenInfo)); });
    await userEvent.setup().click(await screen.findByTestId("pr-more-button"));
    expect(onOpenInfo).toHaveBeenCalledTimes(1);
    expect(onClick).not.toHaveBeenCalled();
  });

  it("surface_should_showTheLegacySingleBadge_When_ReaderUiFlagOff", async () => {
    mockFlags = flags(false);
    await act(async () => { render(renderSurface(makeSession({ pullRequests: threePrs() }))); });
    expect(await screen.findByRole("link", { name: /View GitHub Pull Request #7/ })).toBeInTheDocument();
    expect(screen.queryByTestId("pr-more-button")).not.toBeInTheDocument();
    expect(screen.queryByTestId("pr-chip")).not.toBeInTheDocument();
  });

  it("surface_should_fallBackToLegacyBadgeWithoutConsoleErrors_When_FlagOnAndListEmpty", async () => {
    mockFlags = flags(true);
    const error = jest.spyOn(console, "error").mockImplementation(() => {});
    await act(async () => { render(renderSurface(makeSession({ pullRequests: [] }))); });
    expect(screen.getByRole("link", { name: /View GitHub Pull Request #7/ })).toBeInTheDocument();
    expect(screen.queryByTestId("pr-more-button")).not.toBeInTheDocument();
    const real = error.mock.calls.filter((c) => !String(c[0]).includes("className"));
    expect(real).toEqual([]);
    error.mockRestore();
  });
});

describe("SessionCard body Pull Request row", () => {
  it("card_should_linkTheActivePrInTheBody_When_FlagOn_AndTheLegacyPrWhenOff", async () => {
    mockFlags = flags(true);
    const { unmount } = render(<SessionCard session={makeSession({ pullRequests: threePrs() })} />);
    expect(await screen.findByRole("link", { name: "Pull request #12 on acme/app, opens in new tab" })).toBeInTheDocument();
    unmount();
    mockFlags = flags(false);
    render(<SessionCard session={makeSession({ pullRequests: threePrs() })} />);
    expect(await screen.findByRole("link", { name: "Pull request #7 on acme/app, opens in new tab" })).toBeInTheDocument();
  });
});
