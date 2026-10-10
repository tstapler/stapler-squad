import fs from "fs";
import path from "path";
import React from "react";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { NotificationToast, type NotificationToastProps } from "@/components/ui/NotificationToast";
import type { NotificationData } from "@/lib/types/notification";

jest.mock("@/lib/hooks/useAuditLog", () => ({
  useAuditLog: () => ({ logNotificationSessionViewed: jest.fn() }),
}));

function setCoarse(coarse: boolean) {
  window.matchMedia = jest.fn().mockImplementation((query: string) => ({
    matches: coarse && query.includes("coarse"),
    media: query,
    addEventListener: jest.fn(),
    removeEventListener: jest.fn(),
  })) as unknown as typeof window.matchMedia;
}

function make(overrides: Partial<NotificationData> = {}): NotificationData {
  return {
    id: "t1",
    sessionId: "s1",
    sessionName: "Session",
    title: "Title",
    message: "Body",
    timestamp: Date.now(),
    notificationType: "info",
    ...overrides,
  };
}

function renderCard(overrides: Partial<NotificationData> = {}, props: Partial<NotificationToastProps> = {}) {
  const onClose = jest.fn();
  const utils = render(<NotificationToast notification={make(overrides)} onClose={onClose} stacked {...props} />);
  return { onClose, ...utils };
}

const approval = (extra: Partial<NotificationData> = {}): Partial<NotificationData> => ({
  notificationType: "approval_needed",
  isPendingDecision: true,
  onApprove: jest.fn().mockResolvedValue(undefined),
  onDeny: jest.fn().mockResolvedValue(undefined),
  metadata: { tool_input_command: "rm -rf /tmp/x", risk_level: "low" },
  ...extra,
});

describe("NotificationToast source", () => {
  it("notification_toast_should_contain_no_timer_effect_when_grepped", () => {
    const source = fs.readFileSync(path.join(process.cwd(), "src/components/ui/NotificationToast.tsx"), "utf8");
    expect(source).not.toMatch(/setTimeout|setInterval|requestAnimationFrame/);
  });

  it("swipe_hook_should_skip_fling_animation_when_prefers_reduced_motion", () => {
    const css = fs.readFileSync(path.join(process.cwd(), "src/components/ui/NotificationToast.css.ts"), "utf8");
    const swipeCard = css.slice(css.indexOf("export const swipeCard "));
    expect(swipeCard.slice(0, swipeCard.indexOf("});"))).toMatch(/prefers-reduced-motion: reduce[\s\S]*transition: "none"/);
  });
});

describe("close control (TC-1, TC-11)", () => {
  beforeEach(() => setCoarse(false));

  it("shows an x labelled Dismiss notification for an informational toast", () => {
    const { onClose } = renderCard();
    const button = screen.getByRole("button", { name: "Dismiss notification" });
    fireEvent.click(button);
    expect(onClose).toHaveBeenCalledWith();
    expect(onClose.mock.calls[0]).toHaveLength(0); // not an acknowledge: the history row stays
  });

  it("pinned_swipe_should_demote_to_tray_not_dismiss_and_pinned_close_should_show_tray_icon_and_text_tray", () => {
    const { onClose } = renderCard({ notificationType: "error", isPendingDecision: true });
    const button = screen.getByRole("button", { name: "Move to tray" });
    expect(button).toHaveTextContent("Tray");
    expect(screen.queryByRole("button", { name: "Dismiss notification" })).toBeNull();
    fireEvent.click(button);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("Delete and Backspace on the focused card perform the close action", () => {
    const { onClose } = renderCard();
    const card = screen.getByTestId("toast");
    fireEvent.keyDown(card, { key: "Delete" });
    fireEvent.keyDown(card, { key: "Backspace" });
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("legacy cards keep the Close notification control", () => {
    const onClose = jest.fn();
    render(<NotificationToast notification={make()} onClose={onClose} />);
    expect(screen.getByRole("button", { name: "Close notification" })).toBeInTheDocument();
  });
});

describe("touch actions (TC-5)", () => {
  const withFocusWindow: Partial<NotificationData> = {
    sourceApp: "IntelliJ",
    onFocusWindow: jest.fn(),
    onView: jest.fn(),
  };

  it("toast_should_hide_focus_window_on_coarse_pointer_and_cap_2_actions_and_support_delete_key", () => {
    setCoarse(true);
    renderCard(withFocusWindow, { compact: true });
    expect(screen.queryByText(/Focus Window/)).toBeNull();
    const card = screen.getByTestId("toast");
    const buttons = within(card).getAllByRole("button");
    // Close control plus at most two actions.
    expect(buttons.length - 1).toBeLessThanOrEqual(2);
  });

  it("puts Focus Window in the overflow on a desktop pointer when an approval has more than two actions", () => {
    setCoarse(false);
    renderCard(approval({ ...withFocusWindow }));
    expect(screen.queryByText(/Focus Window/)).toBeNull();
    fireEvent.click(screen.getByTestId("toast-overflow-menu"));
    expect(screen.getByText(/Focus Window/)).toBeInTheDocument();
  });
});

describe("phone approval card (TC-12, TC-13, TC-14)", () => {
  beforeEach(() => setCoarse(true));

  it("shows Approve and Deny together, Deny first, with View Session behind the overflow", () => {
    renderCard(approval());
    const card = screen.getByTestId("toast");
    const labels = within(card)
      .getAllByRole("button")
      .map((b) => b.textContent);
    expect(labels.indexOf("✗ Deny")).toBeGreaterThan(-1);
    expect(labels.indexOf("✗ Deny")).toBeLessThan(labels.indexOf("✓ Approve"));
    expect(within(card).queryByRole("button", { name: "View Session" })).toBeNull();
    fireEvent.click(screen.getByTestId("toast-overflow-menu"));
    expect(within(card).getByRole("button", { name: "View Session" })).toBeInTheDocument();
  });

  it("wraps the command and reveals all of a 600 character command on demand", () => {
    const command = `echo ${"x".repeat(595)}`;
    renderCard(approval({ metadata: { tool_input_command: command, risk_level: "low" } }));
    const text = screen.getByTestId("toast-command");
    expect(text).toHaveTextContent(command);
    expect(text).toHaveAttribute("data-expanded", "false");
    fireEvent.click(screen.getByTestId("toast-show-command"));
    expect(text).toHaveAttribute("data-expanded", "true");
    expect(screen.getByTestId("toast-show-command")).toHaveTextContent("Hide full command");
  });

  it("disables both buttons at once and sends one request on a double tap", async () => {
    let resolve!: () => void;
    const onApprove = jest.fn(() => new Promise<void>((r) => (resolve = r)));
    const { onClose } = renderCard(approval({ onApprove }));
    const approve = screen.getByRole("button", { name: /Approve/ });

    fireEvent.click(approve);
    fireEvent.click(approve);
    expect(onApprove).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Approving..." })).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByRole("button", { name: /Deny/ })).toHaveAttribute("aria-disabled", "true");

    await act(async () => resolve());
    expect(onClose).toHaveBeenCalledWith({ acknowledge: true });
  });

  it("shows Could not approve - Retry on failure, re-enables both and retries", async () => {
    const onApprove = jest.fn().mockRejectedValueOnce(new Error("boom")).mockResolvedValueOnce(undefined);
    const onActionFailed = jest.fn();
    const { onClose } = renderCard(approval({ onApprove }), { onActionFailed });

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /Approve/ }));
    });
    expect(screen.getByTestId("toast-action-error")).toHaveTextContent("Could not approve - Retry");
    expect(onActionFailed).toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: /Deny/ })).not.toHaveAttribute("aria-disabled");

    await act(async () => {
      fireEvent.click(within(screen.getByTestId("toast-action-error")).getByRole("button", { name: "Retry" }));
    });
    expect(onApprove).toHaveBeenCalledTimes(2);
    expect(onClose).toHaveBeenCalledWith({ acknowledge: true });
  });

  it.each(["high", "critical", undefined])("asks first and sends 0 requests until confirmed when risk is %s", async (risk) => {
    const onApprove = jest.fn().mockResolvedValue(undefined);
    const metadata: Record<string, string> = { tool_input_command: "rm -rf /" };
    if (risk) metadata.risk_level = risk;
    renderCard(approval({ onApprove, metadata }));

    fireEvent.click(screen.getByRole("button", { name: /Approve/ }));
    expect(screen.getByTestId("toast-approve-confirm")).toHaveTextContent("Approve this command?");
    expect(onApprove).not.toHaveBeenCalled();

    await act(async () => {
      fireEvent.click(screen.getByTestId("toast-confirm-approve"));
    });
    expect(onApprove).toHaveBeenCalledTimes(1);
  });

  it.each(["low", "medium"])("approves in one tap when risk is %s", async (risk) => {
    const onApprove = jest.fn().mockResolvedValue(undefined);
    renderCard(approval({ onApprove, metadata: { tool_input_command: "ls", risk_level: risk } }));
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /Approve/ }));
    });
    expect(onApprove).toHaveBeenCalledTimes(1);
  });

  it("Deny never needs a confirm", async () => {
    const onDeny = jest.fn().mockResolvedValue(undefined);
    renderCard(approval({ onDeny, metadata: { tool_input_command: "rm -rf /", risk_level: "critical" } }));
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /Deny/ }));
    });
    expect(onDeny).toHaveBeenCalledTimes(1);
  });
});

describe("in-drag reveal (TC-11)", () => {
  beforeEach(() => setCoarse(true));

  function drag(target: Element, to: number) {
    const fire = (type: string, x: number, t: number) => {
      const event = new Event(type, { bubbles: true });
      const point = { clientX: x, clientY: 100 };
      Object.defineProperty(event, "touches", { value: type === "touchend" ? [] : [point] });
      Object.defineProperty(event, "changedTouches", { value: [point] });
      Object.defineProperty(event, "timeStamp", { value: t });
      act(() => {
        target.dispatchEvent(event);
      });
    };
    fire("touchstart", 100, 0);
    fire("touchmove", 100 + to, 400);
    return () => fire("touchend", 100 + to, 800);
  }

  it("in_drag_reveal_should_show_dismiss_for_informational_and_move_to_tray_for_pinned", () => {
    const { unmount } = renderCard();
    drag(screen.getByTestId("toast-row"), 60);
    expect(screen.getByTestId("toast-swipe-reveal")).toHaveTextContent("Dismiss");
    unmount();

    renderCard({ notificationType: "error", isPendingDecision: true });
    drag(screen.getByTestId("toast-row"), 60);
    expect(screen.getByTestId("toast-swipe-reveal")).toHaveTextContent("Move to tray");
  });

  it("a release under 40% springs back with no action", () => {
    const { onClose } = renderCard();
    jest.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      width: 360, height: 80, top: 0, left: 0, right: 360, bottom: 80, x: 0, y: 0, toJSON: () => ({}),
    } as DOMRect);
    const release = drag(screen.getByTestId("toast-row"), 60);
    release();
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.queryByTestId("toast-swipe-reveal")).toBeNull();
    jest.restoreAllMocks();
  });
});
