import React from "react";
import { act, render, screen, within, fireEvent } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import {
  FailingCheckSchema,
  LinkedSessionStatus,
} from "@/gen/session/v1/types_pb";
import { PRCard } from "./PRCard";
import { NudgeOutcome } from "@/gen/session/v1/github_user_pb";
import { failingPR, fakeNudgeClient, makePR, session } from "./prTestFixtures";

jest.mock("./PRCard.css", () => new Proxy({}, { get: (_t, prop) => (typeof prop === "string" ? prop : "") }));
jest.mock("./NudgeButton.css", () => new Proxy({}, { get: (_t, prop) => (typeof prop === "string" ? prop : "") }));

describe("PRCard chips", () => {
  it("prCard_should_RenderChipsFromDetail_When_FailingChecksThreadsConflictPresent", () => {
    const pr = makePR({
      checkConclusion: "failure",
      failingChecks: [
        create(FailingCheckSchema, { name: "lint", url: "https://ci.example/lint", conclusion: "failure" }),
        create(FailingCheckSchema, { name: "unit-1", url: "https://ci.example/unit-1", conclusion: "failure" }),
      ],
      unresolvedThreadCount: 3,
      hasMergeConflict: true,
    });
    render(<PRCard pr={pr} />);

    const toggle = screen.getByRole("button", { name: /^CI: 2 failing/ });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("link", { name: "lint" })).toBeNull();

    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    const lint = screen.getByRole("link", { name: "lint" });
    expect(lint).toHaveAttribute("href", "https://ci.example/lint");
    expect(lint).toHaveAttribute("rel", "noopener noreferrer");
    expect(screen.getByRole("link", { name: "unit-1" })).toBeInTheDocument();

    expect(screen.getByText("3 unresolved")).toBeInTheDocument();
    expect(screen.getByText("Merge conflict")).toBeInTheDocument();
  });

  it("prCard_should_RenderQuestionThreadsNotZero_When_DetailsNotLoaded", () => {
    const { rerender } = render(
      <PRCard pr={makePR({ detailsLoaded: false, unresolvedThreadCount: undefined, hasMergeConflict: undefined })} />
    );
    expect(screen.getByText("? threads")).toBeInTheDocument();
    expect(screen.queryByText(/unresolved/)).toBeNull();

    rerender(<PRCard pr={makePR({ detailsLoaded: true, unresolvedThreadCount: 0 })} />);
    expect(screen.queryByText("? threads")).toBeNull();
    expect(screen.queryByText(/unresolved/)).toBeNull();
  });

  it("prCard_should_RenderFiftyPlusUnresolvedChip_When_UnresolvedThreadsTruncated", () => {
    render(<PRCard pr={makePR({ unresolvedThreadCount: 50, unresolvedThreadsTruncated: true })} />);
    expect(screen.getByText("50+ unresolved")).toBeInTheDocument();
  });

  it("prCard_should_LinkUnresolvedChipToPRPageAndShowHostAccountLabelOnlyWithMultipleHosts_When_Rendered", () => {
    const pr = makePR({ unresolvedThreadCount: 3, host: "ghe.corp", accountLogin: "bob" });
    const { rerender } = render(<PRCard pr={pr} showHostAccount />);
    const chip = screen.getByRole("link", { name: "3 unresolved" });
    expect(chip).toHaveAttribute("href", pr.htmlUrl);
    expect(chip).toHaveAttribute("target", "_blank");
    expect(chip).toHaveAttribute("rel", "noopener noreferrer");
    expect(screen.getByTestId("pr-host-account")).toHaveTextContent("ghe.corp - bob");

    rerender(<PRCard pr={pr} showHostAccount={false} />);
    expect(screen.queryByTestId("pr-host-account")).toBeNull();
  });

  it("prCard_should_ConveyEveryChipByText_When_AllStatesRendered", () => {
    const { container } = render(
      <>
        <PRCard
          pr={makePR({
            number: 1,
            checkConclusion: "failure",
            changesReqCount: 1,
            unresolvedThreadCount: 2,
            hasMergeConflict: true,
          })}
        />
        <PRCard pr={makePR({ number: 2, isDraft: true })} />
        <PRCard pr={makePR({ number: 3, checkConclusion: "pending", approvedCount: 1 })} />
      </>
    );
    for (const text of ["CI: 1 failing", "Changes requested", "2 unresolved", "Merge conflict", "Draft", "CI pending", "1 approved"]) {
      expect(within(container).getByText(text)).toBeInTheDocument();
    }
  });

  it("prCard_should_NotCountPendingAsFailing_When_CheckPending", () => {
    render(<PRCard pr={makePR({ checkConclusion: "pending" })} />);
    expect(screen.queryByRole("button", { name: /failing/ })).toBeNull();
  });
});

describe("PRCard sessions", () => {
  it("prCard_should_ListEveryLinkedSessionWithDefaultLabelAndOpenLink_When_TwoSessions", () => {
    const pr = makePR({
      linkedSessions: [
        session("review-bot", LinkedSessionStatus.PAUSED, 100),
        session("fix-ci", LinkedSessionStatus.RUNNING, 200),
      ],
    });
    render(<PRCard pr={pr} />);
    const rows = within(screen.getByRole("list", { name: "Linked sessions" })).getAllByRole("listitem");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("fix-ci");
    expect(rows[0]).toHaveTextContent("running, default");
    expect(rows[1]).toHaveTextContent("review-bot");
    expect(rows[1]).toHaveTextContent("paused");
    expect(rows[1]).not.toHaveTextContent("default");
    const links = screen.getAllByRole("link", { name: /^Open session/ });
    expect(links.map((l) => l.getAttribute("href"))).toEqual(["/?session=fix-ci", "/?session=review-bot"]);
    expect(screen.queryByText("+ Session")).toBeNull();
  });

  it("prCard_should_ShowNoSessionOnBranchAndPlusSession_When_NoLinkedSessions", () => {
    render(<PRCard pr={makePR()} />);
    expect(screen.getByText("No session on this branch")).toBeInTheDocument();
    expect(screen.getByTestId("create-session-button")).toHaveTextContent("+ Session");
    expect(screen.queryByRole("list", { name: "Linked sessions" })).toBeNull();
  });

  it("prCard_should_FallBackToSessionIds_When_LinkedSessionsEmpty", () => {
    render(<PRCard pr={makePR({ sessionIds: ["legacy-one"] })} />);
    expect(screen.getByRole("link", { name: "Open session legacy-one" })).toBeInTheDocument();
    expect(screen.getByText(/status unknown, default/)).toBeInTheDocument();
  });
});

describe("PRCard changes requested only", () => {
  const changesOnly = () => makePR({ changesReqCount: 1 });

  it("prCard_should_ShowExplanationAndOpenPROnGitHubLinkAndNoAskButton_When_ChangesRequestedOnly", () => {
    render(<PRCard pr={changesOnly()} />);
    expect(
      screen.getByText(
        "A reviewer asked for changes. Open the PR on GitHub to read them; no automatic request is available."
      )
    ).toBeInTheDocument();
    const link = screen.getByRole("link", { name: "Open PR on GitHub" });
    expect(link).toHaveAttribute("href", "https://github.com/acme/api/pull/42");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    expect(screen.queryByRole("button", { name: /^Ask / })).toBeNull();
  });

  it("prCard_should_HideNudgeButNotChip_When_OnlyChangesRequested", () => {
    render(<PRCard pr={makePR({ changesReqCount: 1, linkedSessions: [session("fix-ci", LinkedSessionStatus.RUNNING, 200)] })} />);
    expect(screen.getByText("Changes requested")).toBeInTheDocument();
    expect(screen.queryByTestId("pr-nudge-42")).toBeNull();
    expect(screen.queryByRole("button", { name: /^Ask / })).toBeNull();
  });

  it("does not show the explanation when something is also nudgeable", () => {
    render(<PRCard pr={makePR({ changesReqCount: 1, checkConclusion: "failure" })} />);
    expect(screen.queryByText(/A reviewer asked for changes/)).toBeNull();
  });
});

describe("PRCard nudge", () => {
  const RUN = LinkedSessionStatus.RUNNING;
  const PAUSE = LinkedSessionStatus.PAUSED;
  const nudge = () => screen.queryByTestId("pr-nudge-42");

  it("prCard_should_ShowNudgeOnlyWhenNudgeableAndRunnableSession_When_GreenDraftChangesRequestedOnlyPausedOnly", () => {
    const linked = [session("fix-ci", RUN, 200)];
    const cases: Array<[string, Parameters<typeof makePR>[0], boolean]> = [
      ["failing + running", { checkConclusion: "failure", linkedSessions: linked }, true],
      ["conflict only", { hasMergeConflict: true, linkedSessions: linked }, true],
      ["green", { linkedSessions: linked }, false],
      ["green approved", { approvedCount: 2, linkedSessions: linked }, false],
      ["draft failing", { isDraft: true, checkConclusion: "failure", linkedSessions: linked }, false],
      ["changes requested only", { changesReqCount: 1, linkedSessions: linked }, false],
      ["failing, no session", { checkConclusion: "failure" }, false],
      ["failing, legacy sessionIds only", { checkConclusion: "failure", sessionIds: ["legacy"] }, false],
    ];
    for (const [name, over, expected] of cases) {
      const { unmount } = render(<PRCard pr={makePR(over)} />);
      expect([name, nudge() !== null]).toEqual([name, expected]);
      unmount();
    }
  });

  it("prCard_should_HideNudge_When_GreenApprovedDraftOrChangesRequestedOnly", () => {
    const linked = [session("fix-ci", RUN, 200)];
    for (const over of [{}, { approvedCount: 1 }, { isDraft: true, checkConclusion: "failure" }, { changesReqCount: 1 }]) {
      const { unmount } = render(<PRCard pr={makePR({ linkedSessions: linked, ...over })} />);
      expect(nudge()).toBeNull();
      unmount();
    }
  });

  it("prCard_should_ShowDisabledNudgeReasonTextAndSessionLink_When_OnlyPausedSessions", () => {
    render(<PRCard pr={failingPR({ linkedSessions: [session("review-bot", PAUSE, 100)] })} />);
    expect(nudge()).toHaveAttribute("aria-disabled", "true");
    const reason = screen.getByTestId("nudge-blocked-reason");
    expect(reason).toHaveTextContent("review-bot: Session paused. Open it to resume");
    expect(within(reason).getByRole("link", { name: "Open session review-bot" })).toHaveAttribute("href", "/?session=review-bot");
  });

  it("prCard_should_MoveFocusToStatusRegionNotBody_When_FocusedNudgeButtonDisappears", () => {
    const { rerender } = render(<PRCard pr={failingPR()} />);
    act(() => nudge()!.focus());
    expect(document.activeElement).toBe(nudge());

    rerender(<PRCard pr={failingPR({ checkConclusion: "success" })} />);
    expect(nudge()).toBeNull();
    const region = screen.getByTestId("pr-nudge-status-42");
    expect(document.activeElement).toBe(region);
    expect(document.activeElement).not.toBe(document.body);
    expect(region).toHaveAttribute("tabindex", "-1");
    expect(region).toHaveTextContent("Nothing to fix right now");
  });

  it("prCard_should_LeaveFocusAlone_When_UnfocusedNudgeButtonDisappears", () => {
    const { rerender } = render(<PRCard pr={failingPR()} />);
    rerender(<PRCard pr={failingPR({ checkConclusion: "success" })} />);
    expect(document.activeElement).toBe(document.body);
  });

  it("prCard_should_KeepFocusOnLiveElement_When_NudgeCompletesOrListRefreshes", async () => {
    const client = fakeNudgeClient({ outcome: NudgeOutcome.DELIVERED });
    const { rerender } = render(<PRCard pr={failingPR()} nudgeClient={client} />);
    act(() => nudge()!.focus());
    await act(async () => void fireEvent.click(nudge()!));
    expect(document.activeElement).toBe(nudge());
    rerender(<PRCard pr={failingPR()} nudgeClient={client} />);
    expect(document.activeElement).toBe(nudge());
  });

  it("prCard_should_ShipPlainPlusSession_When_Task142aOutcomeC", () => {
    const cases: Array<Parameters<typeof makePR>[0]> = [
      { checkConclusion: "failure" },
      { changesReqCount: 1 },
      { isDraft: true, checkConclusion: "failure" },
      {},
    ];
    for (const over of cases) {
      const pr = makePR(over);
      const { unmount } = render(<PRCard pr={pr} />);
      const plus = screen.getByTestId("create-session-button");
      expect(plus).toHaveTextContent(/^\+ Session$/);
      expect(plus).toHaveAttribute("href", `/?pr=${encodeURIComponent(pr.htmlUrl)}`);
      expect(nudge()).toBeNull();
      unmount();
    }
  });
});
