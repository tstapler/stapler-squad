import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { SessionSchema, UnfinishedWorktreeSchema } from "@/gen/session/v1/types_pb";
import type { FeatureFlag } from "@/gen/session/v1/session_pb";
import { SessionServiceContext, type SessionServiceContextValue } from "@/lib/contexts/SessionServiceContext";
import { makePr } from "@/lib/pr/__testUtils__/pullRequests";
import { UnfinishedItemDetail } from "./UnfinishedItemDetail";

const pushMock = jest.fn();
jest.mock("@/lib/nav", () => ({
  useRouter: jest.fn(() => ({ push: pushMock })),
  replaceHash: jest.fn(),
}));
jest.mock("@connectrpc/connect", () => ({ createClient: jest.fn(() => ({})) }));
jest.mock("@connectrpc/connect-web", () => ({ createConnectTransport: jest.fn(() => ({ unary: jest.fn(), stream: jest.fn() })) }));
jest.mock("./CommitPushModal", () => ({ CommitPushModal: () => null }));
jest.mock("./WorktreeDiffModal", () => ({ WorktreeDiffModal: () => null }));
let mockFlags: FeatureFlag[] = [];
jest.mock("@/lib/features/flagStore", () => ({
  loadFlags: () => Promise.resolve(mockFlags),
  peekFlags: () => mockFlags,
  subscribeFlags: () => () => {},
}));

const flags = (on: boolean) =>
  [{ name: "session_multi_pr", enabled: false, scopes: ["reader:poller", "reader:backlog", "reader:actions", "reader:ui"].map((scope) => ({ scope, enabled: on })) }] as unknown as FeatureFlag[];

const service = (sessions: ReturnType<typeof create<typeof SessionSchema>>[]) => ({ sessions, loading: false, hasLoadedOnce: true }) as unknown as SessionServiceContextValue;

const worktree = () =>
  create(UnfinishedWorktreeSchema, { sessionIds: ["s1"], githubPrNumber: 5, githubPrUrl: "https://github.com/acme/app/pull/5", githubPrState: "open" });
const sessions = () => [create(SessionSchema, { id: "s1", title: "one", pullRequests: [makePr({ number: 12, isActive: true }), makePr({ number: 13 })] })];

describe("UnfinishedItemDetail PR badge", () => {
  beforeEach(() => pushMock.mockClear());

  it("UnfinishedItemDetail_should_showTheSessionPrChipWithPlusN_When_MultiOnAndTheSessionListsPrs", async () => {
    mockFlags = flags(true);
    render(<SessionServiceContext.Provider value={service(sessions())}><UnfinishedItemDetail worktree={worktree()} /></SessionServiceContext.Provider>);
    expect(await screen.findByTestId("pr-chip")).toHaveTextContent("#12");
    fireEvent.click(screen.getByTestId("pr-more-button"));
    expect(pushMock).toHaveBeenCalledWith("/?session=s1");
  });

  it("UnfinishedItemDetail_should_showTheLegacyBadge_When_FlagOff", async () => {
    mockFlags = flags(false);
    render(<SessionServiceContext.Provider value={service(sessions())}><UnfinishedItemDetail worktree={worktree()} /></SessionServiceContext.Provider>);
    await waitFor(() => expect(screen.queryByTestId("pr-more-button")).toBeNull());
    expect(screen.queryByTestId("pr-chip-group")).toBeNull();
    expect(screen.getByRole("link", { name: /#5/ })).toBeInTheDocument();
  });
});
