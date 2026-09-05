import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { create, type MessageInitShape } from "@bufbuild/protobuf";
import {
  WasteFindingSchema,
  SessionTokenSummarySchema,
  Severity,
  FindingType,
} from "@/gen/session/v1/insights_pb";
import { FindingsPanel } from "./FindingsPanel";

function makeFinding(overrides: MessageInitShape<typeof WasteFindingSchema> = {}) {
  return create(WasteFindingSchema, {
    findingType: FindingType.CACHE_HIT_FLOOR_BREACH,
    severity: Severity.CRITICAL,
    dollarImpactUsd: 4.2,
    sessionId: "session-1",
    conversationId: "conv-1",
    message: "Cache hit rate 9% is below the 40% floor over 6 turns — an estimated $4.20 in avoidable input-token cost.",
    ...overrides,
  });
}

function makeSession(overrides: MessageInitShape<typeof SessionTokenSummarySchema> = {}) {
  return create(SessionTokenSummarySchema, {
    sessionId: "session-1",
    conversationId: "conv-1",
    unpricedModels: [],
    ...overrides,
  });
}

describe("FindingsPanel", () => {
  it("FindingsPanel_should_showSkeleton_when_loading", () => {
    render(
      <FindingsPanel findings={undefined} sessions={undefined} loading={true} error={null} onSessionClick={jest.fn()} />
    );

    expect(screen.getByTestId("findings-skeleton")).toBeInTheDocument();
  });

  it("FindingsPanel_should_showComputedEmptyText_when_findingsArrayIsEmpty", () => {
    const sessions = [makeSession({ unpricedModels: [] })];

    render(<FindingsPanel findings={[]} sessions={sessions} loading={false} error={null} onSessionClick={jest.fn()} />);

    expect(screen.getByText(/no waste patterns detected/i)).toBeInTheDocument();
  });

  it("FindingsPanel_should_showErrorBoxWithRetry_when_parentErrorStateIsSet", async () => {
    const onRetry = jest.fn();
    const user = userEvent.setup();
    render(
      <FindingsPanel
        findings={[]}
        sessions={[]}
        loading={false}
        error="internal error"
        onSessionClick={jest.fn()}
        onRetry={onRetry}
      />
    );

    expect(screen.getByRole("alert")).toHaveTextContent(/couldn't compute findings/i);
    expect(screen.getByRole("alert")).toHaveTextContent(/internal error/i);
    expect(screen.queryByText(/no waste patterns detected/i)).not.toBeInTheDocument();

    const retryButton = screen.getByRole("button", { name: "Retry" });
    expect(retryButton).toBeInTheDocument();

    await user.click(retryButton);
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  // Pre-mortem Failure #1: an all-unpriced-model dashboard (findings.length === 0,
  // every session's unpricedModels.length > 0) must render the unpriced count text,
  // never the "No waste patterns detected" clean-state text — the two empty-looking
  // states must render different strings.
  it("FindingsPanel_should_showUnpricedCount_when_allSessionsUnpricedAndFindingsEmpty", () => {
    const sessions = [
      makeSession({ sessionId: "s1", unpricedModels: ["gpt-99"] }),
      makeSession({ sessionId: "s2", unpricedModels: ["gpt-99"] }),
    ];

    render(<FindingsPanel findings={[]} sessions={sessions} loading={false} error={null} onSessionClick={jest.fn()} />);

    expect(screen.getByText(/2 sessions could not be evaluated \(unpriced model\)/i)).toBeInTheDocument();
    expect(screen.queryByText(/no waste patterns detected/i)).not.toBeInTheDocument();
  });

  it("FindingsPanel_should_showCleanText_when_noSessionUnpricedAndFindingsEmpty", () => {
    const sessions = [
      makeSession({ sessionId: "s1", unpricedModels: [] }),
      makeSession({ sessionId: "s2", unpricedModels: [] }),
    ];

    render(<FindingsPanel findings={[]} sessions={sessions} loading={false} error={null} onSessionClick={jest.fn()} />);

    expect(screen.getByText(/no waste patterns detected/i)).toBeInTheDocument();
    expect(screen.queryByText(/could not be evaluated/i)).not.toBeInTheDocument();
  });

  it("FindingsPanel_should_renderFindingCard_when_findingsPresent", () => {
    const finding = makeFinding();
    const sessions = [makeSession()];

    render(<FindingsPanel findings={[finding]} sessions={sessions} loading={false} error={null} onSessionClick={jest.fn()} />);

    expect(screen.getByRole("list")).toBeInTheDocument();
    const item = screen.getByRole("listitem");
    expect(item).toBeInTheDocument();
    expect(screen.getByText("Critical")).toBeInTheDocument();
    expect(item.textContent).toContain("4.20");
    expect(screen.getByText(finding.message)).toBeInTheDocument();
  });

  // Epic 1.4, Story 1.4.4c: the per-finding action now navigates straight
  // to the deep-linkable route instead of firing onSessionClick.
  it("FindingsPanel_should_linkToSessionRoute_when_actionRendered", () => {
    const finding = makeFinding({ sessionId: "session-1", conversationId: "conv-1" });
    const session = makeSession();

    render(<FindingsPanel findings={[finding]} sessions={[session]} loading={false} error={null} />);

    const action = screen.getByRole("link", { name: /view session/i });
    expect(action).toHaveAttribute("href", "/insights/session-detail?sessionId=session-1");
  });

  it("FindingsPanel_should_linkUsingConversationId_when_findingSessionIdIsEmptyOrphan", () => {
    const finding = makeFinding({ sessionId: "", conversationId: "conv-999" });
    const session = makeSession({ sessionId: "", conversationId: "conv-999" });

    render(<FindingsPanel findings={[finding]} sessions={[session]} loading={false} error={null} />);

    const action = screen.getByRole("link", { name: /view session/i });
    expect(action).toHaveAttribute("href", "/insights/session-detail?sessionId=conv-999");
  });

  it("FindingsPanel_should_exposeActionAsKeyboardFocusableLink_when_findingCardDisplayed", () => {
    const finding = makeFinding();
    const session = makeSession();

    render(<FindingsPanel findings={[finding]} sessions={[session]} loading={false} error={null} />);

    const action = screen.getByRole("link", { name: /view session/i });
    action.focus();
    expect(action).toHaveFocus();
  });

  it("FindingsPanel_should_renderDistinctText_for_computedEmptyVsUnpricedStates", () => {
    const cleanSessions = [makeSession({ unpricedModels: [] })];
    const { unmount } = render(
      <FindingsPanel findings={[]} sessions={cleanSessions} loading={false} error={null} onSessionClick={jest.fn()} />
    );
    const cleanText = screen.getByText(/no waste patterns detected/i).textContent;
    unmount();

    const unpricedSessions = [makeSession({ unpricedModels: ["gpt-99"] })];
    render(<FindingsPanel findings={[]} sessions={unpricedSessions} loading={false} error={null} onSessionClick={jest.fn()} />);
    const unpricedText = screen.getByText(/could not be evaluated/i).textContent;

    expect(cleanText).not.toEqual(unpricedText);
  });
});
