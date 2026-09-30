import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { SessionDetailDrawer } from "./SessionDetailDrawer";
import { SessionTokenSummarySchema, type SessionTokenSummary } from "@/gen/session/v1/insights_pb";

const mockUseSessionTurnTimeline = jest.fn();

jest.mock("@/lib/hooks/useInsightsService", () => ({
  useSessionTurnTimeline: (conversationId: string | undefined) =>
    mockUseSessionTurnTimeline(conversationId),
}));

// See SessionDetailContent.test.tsx for the rationale on overriding the
// generic .css.ts jest mock with real class-name strings for this module.
jest.mock("./SessionDetailDrawer.css", () => ({
  overlay: "overlay",
  drawer: "drawer",
  drawerHeader: "drawerHeader",
  drawerTitle: "drawerTitle",
  sessionIdChip: "sessionIdChip",
  closeButton: "closeButton",
  section: "section",
  sectionTitle: "sectionTitle",
  metaGrid: "metaGrid",
  metaLabel: "metaLabel",
  metaValue: "metaValue",
  toolsTable: "toolsTable",
  toolsTh: "toolsTh",
  toolsThRight: "toolsThRight",
  toolsTd: "toolsTd",
  toolsTdRight: "toolsTdRight",
  outlierCell: "outlierCell",
  skillList: "skillList",
  skillBadge: "skillBadge",
  emptyState: "emptyState",
  srOnly: "srOnly",
  backlogLink: "backlogLink",
}));

function makeSession(
  overrides: Partial<Omit<SessionTokenSummary, "$typeName" | "$unknown">> = {}
): SessionTokenSummary {
  return create(SessionTokenSummarySchema, {
    sessionId: "session-1",
    conversationId: "conversation-1",
    projectPath: "/test/project",
    primaryModel: "claude-sonnet-4",
    estimatedCostUsd: 0.02,
    messageCount: 3,
    topTools: [],
    skillActivations: [],
    unpricedModels: [],
    ...overrides,
  });
}

describe("SessionDetailDrawer", () => {
  beforeEach(() => {
    mockUseSessionTurnTimeline.mockReset();
    mockUseSessionTurnTimeline.mockReturnValue({ turns: [], loading: false, error: null });
  });

  it("SessionDetailDrawer_should_fetchTurnTimelineByConversationId_when_drawerOpensForSession", () => {
    const session = makeSession({ conversationId: "conversation-42" });
    render(<SessionDetailDrawer session={session} onClose={jest.fn()} />);

    expect(mockUseSessionTurnTimeline).toHaveBeenCalledWith("conversation-42");
  });

  it("SessionDetailDrawer_should_returnNull_when_sessionIsNull", () => {
    const { container } = render(<SessionDetailDrawer session={null} onClose={jest.fn()} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("SessionDetailDrawer_should_renderDialogChrome_when_sessionProvided", () => {
    const session = makeSession();
    render(<SessionDetailDrawer session={session} onClose={jest.fn()} />);

    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(screen.getByRole("button", { name: "Close session details" })).toBeInTheDocument();
  });

  it("SessionDetailDrawer_should_callOnClose_when_escapePressed", () => {
    const onClose = jest.fn();
    const session = makeSession();
    render(<SessionDetailDrawer session={session} onClose={onClose} />);

    fireEvent.keyDown(document, { key: "Escape" });

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("SessionDetailDrawer_should_callOnClose_when_overlayClicked", () => {
    const onClose = jest.fn();
    const session = makeSession();
    render(<SessionDetailDrawer session={session} onClose={onClose} />);

    // The drawer renders via createPortal to document.body, so it's outside
    // RTL's default `container` — query the document directly.
    const overlayEl = document.querySelector(".overlay") as HTMLElement;
    fireEvent.click(overlayEl);

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("SessionDetailDrawer_should_moveFocusToCloseButton_when_mountedWithSession", () => {
    const session = makeSession();
    render(<SessionDetailDrawer session={session} onClose={jest.fn()} />);

    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Close session details" })
    );
  });

  it("SessionDetailDrawer_should_restoreFocusToTrigger_when_sessionClosed", () => {
    const trigger = document.createElement("button");
    trigger.textContent = "open session";
    document.body.appendChild(trigger);
    trigger.focus();
    expect(document.activeElement).toBe(trigger);

    const session = makeSession();
    const { rerender } = render(<SessionDetailDrawer session={session} onClose={jest.fn()} />);

    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Close session details" })
    );

    rerender(<SessionDetailDrawer session={null} onClose={jest.fn()} />);

    expect(document.activeElement).toBe(trigger);
    document.body.removeChild(trigger);
  });

  it("SessionDetailDrawer_should_renderSessionDetailContent_when_sessionProvided", () => {
    const session = makeSession();
    render(<SessionDetailDrawer session={session} onClose={jest.fn()} />);

    // SessionDetailContent's Metadata section is rendered inside the drawer —
    // confirms composition, not re-testing SessionDetailContent's own
    // rendering (covered by SessionDetailContent.test.tsx).
    expect(screen.getByText("Metadata")).toBeInTheDocument();
  });
});
