import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { create, type MessageInitShape } from "@bufbuild/protobuf";
import { SessionSchema, type SessionPullRequest } from "@/gen/session/v1/types_pb";
import { OK } from "@/lib/pr/actionResult";
import { openPullRequestsAnchor } from "@/lib/pr/anchor";
import { makePr } from "@/lib/pr/__testUtils__/pullRequests";
import { SessionPullRequestsSection } from "./SessionPullRequestsSection";

let mockMulti = true;
let mockList: SessionPullRequest[] = [];
jest.mock("@/lib/features/sessionMultiPr", () => ({ useSessionMultiPR: () => mockMulti }));
jest.mock("@/lib/hooks/useSessionPullRequests", () => ({
  useSessionPullRequests: () => ({
    pullRequests: mockList,
    holds: [],
    actions: { refresh: jest.fn(async () => OK), merge: jest.fn(), close: jest.fn(), pin: jest.fn(), detach: jest.fn(), attachByUrl: jest.fn() },
  }),
}));
jest.mock("@/lib/contexts/SessionServiceContext", () => ({ useSessionServiceContext: () => ({ draftPullRequest: jest.fn(), createPullRequest: jest.fn() }) }));
jest.mock("./CreatePullRequestModal", () => ({ CreatePullRequestModal: ({ isOpen }: { isOpen: boolean }) => (isOpen ? <div role="dialog" aria-label="Create pull request" /> : null) }));

const session = (over: MessageInitShape<typeof SessionSchema> = {}) =>
  create(SessionSchema, { id: "s1", title: "refactor-auth", githubOwner: "acme", githubRepo: "app", githubPrNumber: 7, githubPrUrl: "https://github.com/acme/app/pull/7", githubPrState: "open", ...over });

beforeEach(() => { mockMulti = true; mockList = []; window.location.hash = ""; });

describe("SessionPullRequestsSection", () => {
  it("section_should_listThreeRowsWithStateTextAndTheDefaultTargetHint_When_FlagOn", () => {
    mockList = [makePr({ number: 12, isActive: true }), makePr({ number: 11, state: "merged" }), makePr({ number: 10, state: "closed" })];
    render(<SessionPullRequestsSection session={session()} />);
    const section = screen.getByRole("region", { name: "Pull requests (3)" });
    expect(within(section).getAllByRole("listitem")).toHaveLength(3);
    expect(within(section).getByTestId("pr-row-11")).toHaveTextContent("Merged");
    expect(within(section).getByText(/Actions apply to #12 acme\/app/)).toBeInTheDocument();
  });

  it("section_should_showOneRowWithNoRoleChipOrDisclosure_When_MigratedLegacySession", () => {
    mockList = [makePr({ number: 7, isActive: true, role: "primary" })];
    render(<SessionPullRequestsSection session={session()} />);
    expect(screen.getAllByRole("listitem")).toHaveLength(1);
    expect(screen.queryByText("primary")).not.toBeInTheDocument();
    expect(screen.queryByTestId("pr-disclosure")).not.toBeInTheDocument();
  });

  it("section_should_renderTheLegacyLink_When_FlagOff", () => {
    mockMulti = false;
    mockList = [makePr({ number: 12, isActive: true }), makePr({ number: 11 })];
    render(<SessionPullRequestsSection session={session()} />);
    expect(screen.queryByTestId("pr-list")).not.toBeInTheDocument();
    const link = screen.getByRole("link", { name: /#7/ });
    expect(link).toHaveAttribute("href", "https://github.com/acme/app/pull/7");
    expect(screen.getByText("GitHub PR:")).toBeInTheDocument();
  });

  it("section_should_renderNothing_When_FlagOffAndNoLegacyPr", () => {
    mockMulti = false;
    const { container } = render(<SessionPullRequestsSection session={session({ githubPrNumber: 0, githubPrUrl: "" })} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("section_should_offerCreatePullRequest_When_FlagOnAndNoPrs", async () => {
    render(<SessionPullRequestsSection session={session()} />);
    await userEvent.setup().click(screen.getByRole("button", { name: "Create pull request" }));
    expect(screen.getByRole("dialog", { name: "Create pull request" })).toBeInTheDocument();
  });

  it("anchor_should_setTheHashAndFocusTheHeading_When_PlusNIsActivated", () => {
    mockList = [makePr({ number: 12, isActive: true }), makePr({ number: 11 })];
    render(<SessionPullRequestsSection session={session()} />);
    expect(screen.getByRole("heading", { name: "Pull requests (2)" })).not.toHaveFocus();
    act(() => openPullRequestsAnchor());
    expect(window.location.hash).toBe("#pull-requests");
    expect(screen.getByRole("heading", { name: "Pull requests (2)" })).toHaveFocus();
    expect(screen.getByTestId("pr-section")).toHaveAttribute("id", "pull-requests");
  });

  it("anchor_should_focusTheHeadingOnMount_When_SessionOpensAfterThePlusNClick", () => {
    mockList = [makePr({ number: 12, isActive: true }), makePr({ number: 11 })];
    act(() => openPullRequestsAnchor());
    render(<SessionPullRequestsSection session={session()} />);
    expect(screen.getByRole("heading", { name: "Pull requests (2)" })).toHaveFocus();
  });
});
