import { render } from "@testing-library/react";
import { AnnouncerProvider } from "../Announcer";
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
}

let rowCounter = 0;

/** Each render gets its own row id: Deny drafts are keyed by it and outlive a test. */
export function renderRow(props: Partial<ActionRowProps> & Pick<ActionRowProps, "descriptors">, { mobile = false }: HarnessOptions = {}) {
  return render(
    <AnnouncerProvider>
      <DeckViewportContext.Provider value={{ isInnerScreen: !mobile, isVirtualKeyboardOpen: false }}>
        <ActionRow rowId={`row-${++rowCounter}`} {...props} />
      </DeckViewportContext.Provider>
    </AnnouncerProvider>,
  );
}
