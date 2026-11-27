import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Session } from "@/gen/session/v1/types_pb";
import type { LinkedSession } from "@/lib/hooks/useBacklogService";
import { JumpToSessionPanel } from "./JumpToSessionPanel";

jest.mock("@/components/sessions/SessionDetail", () => ({
  SessionDetail: ({ session }: { session: { id: string } }) => (
    <div data-testid="session-detail">{session.id}</div>
  ),
}));
const mockTrack = jest.fn();
jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: mockTrack }),
}));
let mockIsMobile = false;
jest.mock("@/components/providers/ViewportProvider", () => ({
  useViewport: () => ({ isMobile: mockIsMobile }),
}));
jest.mock("@/components/sessions/SessionPeekModal", () => ({
  SessionPeekModal: ({ session, onClose }: { session: { id: string }; onClose: () => void }) => (
    <div data-testid="session-peek-modal">
      {session.id}
      {/* eslint-disable-next-line analytics/require-on-click -- test mock, not production UI */}
      <button onClick={onClose}>close-modal</button>
    </div>
  ),
}));

beforeEach(() => {
  mockIsMobile = false;
  mockTrack.mockClear();
});

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

  it("JumpToSessionPanel_should_ShowRestartWork_When_SessionNotFoundAndHandlerProvided", async () => {
    const user = userEvent.setup();
    const onRestartWork = jest.fn();
    render(
      <JumpToSessionPanel
        linkedSessions={[makeLinkedSession({ sessionId: "w1", role: "work" })]}
        backlogItemId="item1"
        getSession={jest.fn().mockResolvedValue(null)}
        onRestartWork={onRestartWork}
      />
    );
    await user.click(screen.getByTestId("jump-to-session-button"));
    await user.click(await screen.findByRole("button", { name: "Restart work" }));
    expect(onRestartWork).toHaveBeenCalledTimes(1);
  });

  it("JumpToSessionPanel_should_OmitRestartWork_When_NoHandler", async () => {
    const user = userEvent.setup();
    render(
      <JumpToSessionPanel
        linkedSessions={[makeLinkedSession({ sessionId: "w1", role: "work" })]}
        backlogItemId="item1"
        getSession={jest.fn().mockResolvedValue(null)}
      />
    );
    await user.click(screen.getByTestId("jump-to-session-button"));
    await screen.findByText("Session not found.");
    expect(screen.queryByRole("button", { name: "Restart work" })).not.toBeInTheDocument();
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

  describe("role menu position", () => {
    // jsdom has no layout, so the trigger, menu and viewport boxes are stubbed.
    const box = (top: number, left: number, width: number, height: number) =>
      ({ top, left, width, height, bottom: top + height, right: left + width }) as DOMRect;
    let trigger: DOMRect;

    beforeEach(() => {
      jest.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (this: Element) {
        return this.getAttribute("role") === "menu" ? box(0, 0, 200, 100) : trigger;
      });
      Object.defineProperty(document.documentElement, "clientWidth", { configurable: true, value: 320 });
      Object.defineProperty(document.documentElement, "clientHeight", { configurable: true, value: 568 });
    });

    afterEach(() => {
      jest.restoreAllMocks();
      delete (document.documentElement as { clientWidth?: number }).clientWidth;
      delete (document.documentElement as { clientHeight?: number }).clientHeight;
    });

    async function openRoleMenu(wrapper?: React.JSXElementConstructor<{ children: React.ReactNode }>) {
      const user = userEvent.setup();
      render(
        <JumpToSessionPanel
          linkedSessions={[
            makeLinkedSession({ sessionId: "w1", role: "work" }),
            makeLinkedSession({ sessionId: "r1", role: "review" }),
          ]}
          backlogItemId="item1"
          getSession={jest.fn().mockResolvedValue(makeFullSession("w1"))}
        />,
        { wrapper }
      );
      await user.click(screen.getByTestId("jump-to-session-role-menu-trigger"));
      return screen.getByTestId("jump-to-session-role-menu");
    }

    it("JumpToSessionPanel_should_FlipAboveAndStayOnScreen_When_TriggerIsAtBottomRight", async () => {
      trigger = box(520, 270, 44, 44);
      const menu = await openRoleMenu();
      expect(menu).toHaveStyle({ top: "416px", left: "112px" });
    });

    it("JumpToSessionPanel_should_FollowTheTrigger_When_LayoutShiftsWhileMenuIsOpen", async () => {
      trigger = box(300, 20, 44, 44);
      const menu = await openRoleMenu();
      expect(menu).toHaveStyle({ top: "348px", left: "20px" });

      trigger = box(226, 20, 44, 44);
      await waitFor(() => expect(menu).toHaveStyle({ top: "274px" }));
    });

    it("JumpToSessionPanel_should_ClampToBottomEdge_When_TriggerIsBelowViewport", async () => {
      trigger = box(700, 20, 44, 44);
      const menu = await openRoleMenu();
      expect(menu).toHaveStyle({ top: "460px" });
    });

    it("JumpToSessionPanel_should_CloseWithoutStealingFocus_When_FocusLeavesMenu", async () => {
      trigger = box(300, 20, 44, 44);
      const outside = document.body.appendChild(document.createElement("input"));
      await openRoleMenu();

      // Ctrl+K as the Omnibar sees it: a key press, then focus moves.
      fireEvent.keyDown(screen.getAllByRole("menuitem")[0], { key: "k", ctrlKey: true });
      act(() => outside.focus());

      expect(screen.queryByTestId("jump-to-session-role-menu")).not.toBeInTheDocument();
      expect(outside).toHaveFocus();
      outside.remove();
    });

    it("JumpToSessionPanel_should_StayOpenAndKeepFocus_When_SomethingAutofocusesWithoutInput", async () => {
      trigger = box(300, 20, 44, 44);
      const user = userEvent.setup();
      const terminal = document.body.appendChild(document.createElement("textarea"));
      const menu = await openRoleMenu();
      const [first, second] = screen.getAllByRole("menuitem");

      act(() => terminal.focus());
      expect(menu).toBeInTheDocument();
      expect(first).toHaveFocus();

      // Neither Tab nor a click inside the menu counts as the user leaving.
      await user.tab();
      await user.click(menu);
      act(() => terminal.focus());
      expect(menu).toBeInTheDocument();
      expect(first).toHaveFocus();
      expect(second).not.toHaveFocus();
      terminal.remove();
    });

    it("JumpToSessionPanel_should_Close_When_ADialogTakesFocusWithoutInput", async () => {
      trigger = box(300, 20, 44, 44);
      const dialog = document.body.appendChild(document.createElement("div"));
      dialog.setAttribute("role", "alertdialog");
      const confirm = dialog.appendChild(document.createElement("button"));
      await openRoleMenu();

      act(() => confirm.focus());

      expect(screen.queryByTestId("jump-to-session-role-menu")).not.toBeInTheDocument();
      expect(confirm).toHaveFocus();
      dialog.remove();
    });

    it("JumpToSessionPanel_should_CloseFromItsTrigger_When_ClickGivesTheButtonFocus", async () => {
      trigger = box(300, 20, 44, 44);
      const user = userEvent.setup();
      await openRoleMenu();

      await user.click(screen.getByTestId("jump-to-session-role-menu-trigger"));

      expect(screen.queryByTestId("jump-to-session-role-menu")).not.toBeInTheDocument();
    });

    it("JumpToSessionPanel_should_CloseFromItsTrigger_When_ClickFocusesAnAncestorAsSafariDoes", async () => {
      trigger = box(300, 20, 44, 44);
      await openRoleMenu(({ children }) => <main tabIndex={-1}>{children}</main>);
      const menuTrigger = screen.getByTestId("jump-to-session-role-menu-trigger");

      // Safari's sequence: mousedown focuses <main>, not the button, then click fires.
      fireEvent.pointerDown(menuTrigger);
      fireEvent.mouseDown(menuTrigger);
      act(() => screen.getByRole("main").focus());
      fireEvent.mouseUp(menuTrigger);
      fireEvent.click(menuTrigger);

      expect(screen.queryByTestId("jump-to-session-role-menu")).not.toBeInTheDocument();
    });

    it("JumpToSessionPanel_should_ScrollTriggerIntoView_When_OpeningTheMenuOnly", async () => {
      trigger = box(300, 20, 44, 44);
      const scrollIntoView = jest.fn();
      Element.prototype.scrollIntoView = scrollIntoView;
      try {
        const user = userEvent.setup();
        await openRoleMenu();
        expect(scrollIntoView).toHaveBeenCalledTimes(1);
        expect(scrollIntoView.mock.contexts[0]).toBe(screen.getByTestId("jump-to-session-role-menu-trigger"));

        await user.click(screen.getByTestId("jump-to-session-role-menu-trigger"));
        expect(scrollIntoView).toHaveBeenCalledTimes(1);
      } finally {
        delete (Element.prototype as { scrollIntoView?: unknown }).scrollIntoView;
      }
    });

    it("JumpToSessionPanel_should_CycleItemsAndStayOpen_When_TabPressedInMenu", async () => {
      trigger = box(300, 20, 44, 44);
      const user = userEvent.setup();
      const menu = await openRoleMenu();
      const [first, second] = screen.getAllByRole("menuitem");
      expect(first).toHaveFocus();

      await user.tab();
      expect(second).toHaveFocus();
      await user.tab();
      expect(first).toHaveFocus();
      await user.tab({ shift: true });
      expect(second).toHaveFocus();
      expect(menu).toBeInTheDocument();
    });

    it("JumpToSessionPanel_should_ReturnFocusToTrigger_When_EscapePressedOrRoleChosen", async () => {
      trigger = box(300, 20, 44, 44);
      const user = userEvent.setup();
      await openRoleMenu();
      const menuTrigger = screen.getByTestId("jump-to-session-role-menu-trigger");

      await user.keyboard("{Escape}");
      expect(screen.queryByTestId("jump-to-session-role-menu")).not.toBeInTheDocument();
      expect(menuTrigger).toHaveFocus();

      await user.click(menuTrigger);
      await user.click(screen.getByTestId("jump-to-session-role-item-work"));
      expect(menuTrigger).toHaveFocus();
    });

    it("JumpToSessionPanel_should_StillCloseOnEscape_When_MenuPaddingClicked", async () => {
      trigger = box(300, 20, 44, 44);
      const user = userEvent.setup();
      const menu = await openRoleMenu();

      await user.click(menu);
      expect(menu).toHaveFocus();
      await user.keyboard("{Escape}");
      expect(menu).not.toBeInTheDocument();
    });

    it("JumpToSessionPanel_should_CloseMenu_When_TriggerScrollsOutOfView", async () => {
      trigger = box(300, 20, 44, 44);
      let report: (isIntersecting: boolean) => void = () => {};
      const original = window.IntersectionObserver;
      const observe = jest.fn();
      window.IntersectionObserver = jest.fn((callback: IntersectionObserverCallback) => {
        report = (isIntersecting) =>
          callback([{ isIntersecting } as IntersectionObserverEntry], {} as IntersectionObserver);
        return { observe, disconnect: jest.fn() };
      }) as unknown as typeof IntersectionObserver;
      try {
        await openRoleMenu();
        expect(observe).toHaveBeenCalledWith(screen.getByTestId("jump-to-session-role-menu-trigger"));
        act(() => report(true));
        expect(screen.getByTestId("jump-to-session-role-menu")).toBeInTheDocument();

        act(() => report(false));
        expect(screen.queryByTestId("jump-to-session-role-menu")).not.toBeInTheDocument();
        expect(screen.getByTestId("jump-to-session-role-menu-trigger")).toHaveFocus();
      } finally {
        window.IntersectionObserver = original;
      }
    });
  });

  describe("on mobile", () => {
    beforeEach(() => {
      mockIsMobile = true;
    });

    it("JumpToSessionPanel_should_OpenModal_When_MobileAndButtonClicked", async () => {
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

      await waitFor(() => expect(screen.getByTestId("session-peek-modal")).toHaveTextContent("w1"));
      expect(screen.queryByTestId("jump-to-session-panel")).not.toBeInTheDocument();
      expect(screen.queryByTestId("session-detail")).not.toBeInTheDocument();
      expect(mockTrack).toHaveBeenCalledTimes(1);
      expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({ name: "backlog_jump_to_session" }));
    });

    it("JumpToSessionPanel_should_CloseModal_When_MobileAndModalClosed", async () => {
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
      await waitFor(() => expect(screen.getByTestId("session-peek-modal")).toBeInTheDocument());

      await user.click(screen.getByText("close-modal"));

      expect(screen.queryByTestId("session-peek-modal")).not.toBeInTheDocument();
      expect(screen.getByTestId("jump-to-session-button")).toHaveTextContent("Jump to Work session");
      await waitFor(() => expect(screen.getByTestId("jump-to-session-button")).toHaveFocus());
    });

    it("JumpToSessionPanel_should_OpenModalForChosenRole_When_MobileAndRoleChosenFromMenu", async () => {
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

      await user.click(screen.getByTestId("jump-to-session-role-menu-trigger"));
      await user.click(screen.getByTestId("jump-to-session-role-item-review"));

      await waitFor(() => expect(screen.getByTestId("session-peek-modal")).toHaveTextContent("r1"));
      expect(getSession).toHaveBeenCalledTimes(1);
      expect(screen.queryByTestId("jump-to-session-panel")).not.toBeInTheDocument();
      expect(mockTrack).toHaveBeenCalledTimes(1);
    });
  });

  it("JumpToSessionPanel_should_KeepInlineExpandAndNoModal_When_Desktop", async () => {
    const user = userEvent.setup();
    render(
      <JumpToSessionPanel
        linkedSessions={[makeLinkedSession({ sessionId: "w1", role: "work" })]}
        backlogItemId="item1"
        getSession={jest.fn().mockResolvedValue(makeFullSession("w1"))}
      />
    );
    await user.click(screen.getByTestId("jump-to-session-button"));
    await waitFor(() => expect(screen.getByTestId("jump-to-session-panel")).toBeInTheDocument());
    expect(screen.queryByTestId("session-peek-modal")).not.toBeInTheDocument();
  });
});
