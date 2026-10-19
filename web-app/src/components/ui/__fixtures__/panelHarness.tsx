import { render } from "@testing-library/react";
import { AnnouncerContext } from "@/lib/hooks/useAnnounce";
import type { NotificationHistoryItem } from "@/lib/types/notification";
import { NotificationPanel } from "../NotificationPanel";
import { makeApprovalDescriptors, makeNotification } from "./notificationFixtures";

export { panelState, notificationsModule, virtualizerModule } from "./panelMocks";

const ARRIVAL_FROM = 9;

/** Pending approvals n0..n(count-1), newest first; ids and approval ids are `n<i>` and `a<i>`. */
export function pendingApprovals(count: number, startAt = 0): NotificationHistoryItem[] {
  return Array.from({ length: count }, (_, k) => {
    const i = startAt + k;
    return makeNotification({
      id: `n${i}`,
      sessionId: `s${i}`,
      timestamp: i >= ARRIVAL_FROM ? 100_000 - i : 10_000 - i,
      notificationType: "approval_needed",
      isPendingDecision: true,
      metadata: { approval_id: `a${i}`, tool_name: "Bash", tool_input_command: `echo ${i}` },
      actions: makeApprovalDescriptors(`a${i}`),
    });
  });
}

/** The poll result for an approval another client resolved: no longer pending, decision stamped. */
export const resolvedElsewhere = (n: NotificationHistoryItem): NotificationHistoryItem => ({
  ...n,
  isPendingDecision: false,
  isRead: true,
  metadata: { ...n.metadata, approval_decision: "allow" },
});

export const rowKeys = (container: HTMLElement) => Array.from(container.querySelectorAll("[data-tray-row]")).map((el) => el.getAttribute("data-tray-row"));

export function renderPanel() {
  const announce = jest.fn();
  // A fresh element each time: React bails out of re-rendering an identical one.
  const tree = () => (
    <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>
      <NotificationPanel />
    </AnnouncerContext.Provider>
  );
  const view = render(tree());
  return { ...view, announce, refreshPanel: () => view.rerender(tree()) };
}
