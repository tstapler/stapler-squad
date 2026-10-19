import { render } from "@testing-library/react";
import { AnnouncerProvider } from "../Announcer";
import { AnnouncerContext } from "@/lib/hooks/useAnnounce";
import { DeckViewportContext } from "@/lib/contexts/deckViewportContext";
import { ActionRow, type ActionRowProps } from "../notification-actions/ActionRow";
import { invokeNotificationAction } from "@/lib/api/notificationActionApi";
import type { ActionOutcome } from "@/gen/session/v1/notification_action_pb";
import { makeActionResponse } from "./notificationFixtures";

export const invokeMock = invokeNotificationAction as unknown as jest.Mock;

/** Resolves every invoke with `outcome`; call inside `jest.mock("@/lib/api/notificationActionApi")` suites. */
export function respondWith(outcome: ActionOutcome, over: Parameters<typeof makeActionResponse>[1] = {}) {
  invokeMock.mockResolvedValue(makeActionResponse(outcome, over));
}

interface HarnessOptions {
  /** Below 900px (the phone layout). */
  mobile?: boolean;
  /** Replaces the real announcer, to count what the row says. */
  announce?: jest.Mock;
}

let rowCounter = 0;

/** Each render gets its own row id: Deny drafts are keyed by it and outlive a test. */
export function renderRow(props: Partial<ActionRowProps> & Pick<ActionRowProps, "descriptors">, { mobile = false, announce }: HarnessOptions = {}) {
  const rowId = props.rowId ?? `row-${++rowCounter}`;
  const tree = () => (
    <DeckViewportContext.Provider value={{ isInnerScreen: !mobile, isVirtualKeyboardOpen: false }}>
      <ActionRow {...props} rowId={rowId} />
    </DeckViewportContext.Provider>
  );
  const wrapped = () =>
    announce ? <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>{tree()}</AnnouncerContext.Provider> : <AnnouncerProvider>{tree()}</AnnouncerProvider>;
  const view = render(wrapped());
  return { ...view, rowId, /** Re-renders the same row, e.g. after a mocked route changed. */ refreshRow: () => view.rerender(wrapped()) };
}
