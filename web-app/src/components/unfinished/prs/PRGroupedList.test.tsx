import React from "react";
import { render, screen, fireEvent, act, within } from "@testing-library/react";
import { create, type MessageInitShape } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { FailingCheckSchema, UserPRSchema, type UserPR } from "@/gen/session/v1/types_pb";
import { PRGroupedList } from "./PRGroupedList";

jest.mock("./PRCard.css", () => new Proxy({}, { get: (_t, prop) => (typeof prop === "string" ? prop : "") }));
jest.mock("./PRGroupedList.css", () => new Proxy({}, { get: (_t, prop) => (typeof prop === "string" ? prop : "") }));

function makePR(number: number, over: MessageInitShape<typeof UserPRSchema> = {}): UserPR {
  return create(UserPRSchema, {
    failingChecks:
      over.checkConclusion === "failure"
        ? [create(FailingCheckSchema, { name: "lint", url: "", conclusion: "failure" })]
        : [],
    owner: "acme",
    repo: "web",
    number,
    title: `PR ${number}`,
    htmlUrl: `https://github.com/acme/web/pull/${number}`,
    host: "github.com",
    accountLogin: "alice",
    checkConclusion: "success",
    detailsLoaded: true,
    unresolvedThreadCount: 0,
    hasMergeConflict: false,
    updatedAt: timestampFromDate(new Date(number * 1000)),
    ...over,
  });
}

const titles = () => screen.getAllByTestId("github-pr-card").map((c) => c.getAttribute("data-pr-key")!.split("#")[1]);

describe("PRGroupedList", () => {
  it("prGroupedList_should_RenderFailingBeforeGreen_When_SameRepoGroup", () => {
    const green = makePR(1, { approvedCount: 1, updatedAt: timestampFromDate(new Date(9_000_000)) });
    const failing = makePR(2, { checkConclusion: "failure" });
    render(<PRGroupedList prs={[green, failing]} sortBy="attention-first" stateKey="s" />);
    expect(titles()).toEqual(["2", "1"]);
  });

  it("prGroupedList_should_FreezeOrderUpdateContentInPlaceAndApplyOnRefreshList_When_PollChangesOrderOrMembership", () => {
    const b = makePR(2, { checkConclusion: "failure" });
    const a = makePR(1);
    const fallback = { current: document.createElement("h2") };
    document.body.appendChild(fallback.current);
    fallback.current.tabIndex = -1;

    const { rerender } = render(
      <PRGroupedList prs={[a, b]} sortBy="attention-first" stateKey="s" fallbackFocusRef={fallback} />
    );
    expect(titles()).toEqual(["2", "1"]);
    expect(screen.queryByRole("button", { name: "Refresh list" })).toBeNull();
    // the live region exists before it has content
    expect(screen.getByRole("status")).toBeEmptyDOMElement();

    // keyboard focus on card 2 (no hover/pointer dependence)
    const bLink = screen.getAllByRole("link", { name: /PR #2/ })[0];
    act(() => bLink.focus());

    // poll: PR 1 now fails and is newer => live order would flip; PR 3 is new
    const aFailing = makePR(1, { checkConclusion: "failure", updatedAt: timestampFromDate(new Date(9_000_000)) });
    const c = makePR(3, { checkConclusion: "failure", updatedAt: timestampFromDate(new Date(8_000_000)) });
    rerender(<PRGroupedList prs={[aFailing, b, c]} sortBy="attention-first" stateKey="s" fallbackFocusRef={fallback} />);

    expect(titles()).toEqual(["2", "1"]); // order and membership frozen
    const card1 = screen.getAllByTestId("github-pr-card").find((c) => c.dataset.prKey?.endsWith("#1"))!;
    expect(within(card1).getByText("CI: 1 failing")).toBeInTheDocument(); // content updated in place
    expect(screen.getByRole("status")).toHaveTextContent("3 PRs changed.");

    const button = screen.getByRole("button", { name: "Refresh list" });
    fireEvent.touchStart(button);
    fireEvent.click(button);

    expect(titles()).toEqual(["1", "3", "2"]);
    expect(screen.getByRole("status")).toBeEmptyDOMElement();
    expect(document.activeElement).toBe(screen.getAllByRole("link", { name: /PR #2/ })[0]);
  });

  it("prGroupedList_should_ApplyLiveOrder_When_StateKeyChangesOrListRemounts", () => {
    const b = makePR(2, { checkConclusion: "failure" });
    const a = makePR(1);
    const aFailing = makePR(1, { checkConclusion: "failure", updatedAt: timestampFromDate(new Date(9_000_000)) });

    const { rerender, unmount } = render(<PRGroupedList prs={[a, b]} sortBy="attention-first" stateKey="s1" />);
    rerender(<PRGroupedList prs={[aFailing, b]} sortBy="attention-first" stateKey="s1" />);
    expect(titles()).toEqual(["2", "1"]);

    rerender(<PRGroupedList prs={[aFailing, b]} sortBy="attention-first" stateKey="s2" />);
    expect(titles()).toEqual(["1", "2"]);
    expect(screen.getByRole("status")).toBeEmptyDOMElement();

    unmount();
    render(<PRGroupedList prs={[a, aFailing, b].slice(1)} sortBy="attention-first" stateKey="s2" />);
    expect(titles()).toEqual(["1", "2"]);
  });

  it("prGroupedList_should_FollowLiveData_When_FirstPaintWasEmpty", () => {
    const { rerender } = render(<PRGroupedList prs={[]} sortBy="attention-first" stateKey="s" />);
    rerender(<PRGroupedList prs={[makePR(1), makePR(2, { checkConclusion: "failure" })]} sortBy="attention-first" stateKey="s" />);
    expect(titles()).toEqual(["2", "1"]);
    expect(screen.getByRole("status")).toBeEmptyDOMElement();
  });

  it("prGroupedList_should_ShowHostAccountLabelsAndSeparateGroups_When_SameRepoOnTwoHosts", () => {
    const gh = makePR(1);
    const ghe = makePR(2, { host: "ghe.corp", accountLogin: "bob" });
    render(<PRGroupedList prs={[gh, ghe]} sortBy="repo" stateKey="s" />);
    expect(screen.getAllByTestId("pr-host-account").map((e) => e.textContent)).toEqual([
      "ghe.corp - bob",
      "github.com - alice",
    ]);
    expect(screen.getByText("acme/web")).toBeInTheDocument();
    expect(screen.getByText("ghe.corp/acme/web")).toBeInTheDocument();
  });
});
