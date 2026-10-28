import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Session } from "@/gen/session/v1/types_pb";
import type { LinkedSession } from "@/lib/hooks/useBacklogService";
import { JumpToSessionPanel } from "./JumpToSessionPanel";

jest.mock("@/components/sessions/SessionDetail", () => ({
  SessionDetail: ({ session }: { session: { id: string } }) => (
    <div data-testid="session-detail">{session.id}</div>
  ),
}));
jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

function makeLinkedSession(overrides: Partial<LinkedSession> & { sessionId: string; role: string }): LinkedSession {
  return {
    entityId: overrides.sessionId,
    startedAt: "2026-01-01T00:00:00Z",
    endedAt: undefined,
    estimatedCostUsd: 0,
    commitCountSinceSpawn: 0,
    ...overrides,
  } as LinkedSession;
}

function makeFullSession(id: string): Session {
  return { id, title: id } as unknown as Session;
}

describe("JumpToSessionPanel", () => {
  it("JumpToSessionPanel_should_RenderNothing_When_NoJumpableSessionExists", () => {
    const getSession = jest.fn();
    const { container } = render(
      <JumpToSessionPanel
        linkedSessions={[makeLinkedSession({ sessionId: "headless-t1", role: "triage" })]}
        backlogItemId="item1"
        getSession={getSession}
      />
    );
    expect(container).toBeEmptyDOMElement();
    expect(getSession).not.toHaveBeenCalled();
  });

  it("JumpToSessionPanel_should_HideRoleMenu_When_OnlyOneRoleIsJumpable", () => {
    render(
      <JumpToSessionPanel
        linkedSessions={[makeLinkedSession({ sessionId: "w1", role: "work" })]}
        backlogItemId="item1"
        getSession={jest.fn()}
      />
    );
    expect(screen.getByTestId("jump-to-session-button")).toHaveTextContent("Jump to Work session");
    expect(screen.queryByTestId("jump-to-session-role-menu-trigger")).not.toBeInTheDocument();
  });

  it("JumpToSessionPanel_should_FetchAndRenderTerminal_When_ButtonClicked", async () => {
    const user = userEvent.setup();
    const getSession = jest.fn().mockResolvedValue(makeFullSession("w1"));
    render(
      <JumpToSessionPanel
        linkedSessions={[makeLinkedSession({ sessionId: "w1", role: "work" })]}
        backlogItemId="item1"
        getSession={getSession}
      />
    );

    await user.click(screen.getByTestId("jump-to-session-button"));

    expect(getSession).toHaveBeenCalledWith("w1");
    await waitFor(() => expect(screen.getByTestId("session-detail")).toHaveTextContent("w1"));
  });

  it("JumpToSessionPanel_should_ShowError_When_GetSessionReturnsNull", async () => {
    const user = userEvent.setup();
    const getSession = jest.fn().mockResolvedValue(null);
    render(
      <JumpToSessionPanel
        linkedSessions={[makeLinkedSession({ sessionId: "w1", role: "work" })]}
        backlogItemId="item1"
        getSession={getSession}
      />
    );

    await user.click(screen.getByTestId("jump-to-session-button"));

    await waitFor(() => expect(screen.getByText("Session not found.")).toBeInTheDocument());
  });

  it("JumpToSessionPanel_should_CollapsePanel_When_ClickedAgain", async () => {
    const user = userEvent.setup();
    const getSession = jest.fn().mockResolvedValue(makeFullSession("w1"));
    render(
      <JumpToSessionPanel
        linkedSessions={[makeLinkedSession({ sessionId: "w1", role: "work" })]}
        backlogItemId="item1"
        getSession={getSession}
      />
    );

    await user.click(screen.getByTestId("jump-to-session-button"));
    await waitFor(() => expect(screen.getByTestId("jump-to-session-panel")).toBeInTheDocument());

    await user.click(screen.getByTestId("jump-to-session-button"));
    expect(screen.queryByTestId("jump-to-session-panel")).not.toBeInTheDocument();
  });

  it("JumpToSessionPanel_should_SwitchTargetSession_When_DifferentRoleChosenFromMenu", async () => {
    const user = userEvent.setup();
    const getSession = jest.fn().mockImplementation((id: string) => Promise.resolve(makeFullSession(id)));
    render(
      <JumpToSessionPanel
        linkedSessions={[
          makeLinkedSession({ sessionId: "r1", role: "review" }),
          makeLinkedSession({ sessionId: "w1", role: "work" }),
        ]}
        backlogItemId="item1"
        getSession={getSession}
      />
    );

    // Default target is the globally most recent jumpable session (w1, listed last = most recent).
    await user.click(screen.getByTestId("jump-to-session-button"));
    await waitFor(() => expect(getSession).toHaveBeenCalledWith("w1"));

    await user.click(screen.getByTestId("jump-to-session-role-menu-trigger"));
    await user.click(screen.getByTestId("jump-to-session-role-item-review"));

    await waitFor(() => expect(getSession).toHaveBeenCalledWith("r1"));
  });
});
