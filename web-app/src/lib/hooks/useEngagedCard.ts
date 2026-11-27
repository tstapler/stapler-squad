"use client";

import { createContext, useCallback, useContext, useMemo, useRef, useSyncExternalStore, type FocusEvent, type PointerEvent, type TouchEvent } from "react";
import { EngagementTracker } from "./engagementTracker";

/** Provided by the panel; without one (page, stories, unit tests) every card is simply never engaged. */
export const EngagementContext = createContext<EngagementTracker | null>(null);

/** The panel owns one tracker; `frozen` is true while a card is engaged and for 750 ms after. */
export function useEngagementTracker(): { tracker: EngagementTracker; frozen: boolean } {
  const ref = useRef<EngagementTracker | null>(null);
  if (!ref.current) ref.current = new EngagementTracker();
  const tracker = ref.current;
  const frozen = useSyncExternalStore(tracker.subscribe, tracker.isFrozen, () => false);
  return { tracker, frozen };
}

export const useEngagement = (): EngagementTracker | null => useContext(EngagementContext);

/** What a card's controls report back: sub-steps, running holds and resolution. */
export interface CardEngagement {
  setSubStep: (v: boolean) => void;
  setHold: (v: boolean) => void;
  setResolved: (v: boolean) => void;
}

const NOOP_CARD: CardEngagement = { setSubStep: () => {}, setHold: () => {}, setResolved: () => {} };
export const CardEngagementContext = createContext<CardEngagement>(NOOP_CARD);
export const useCardEngagement = (): CardEngagement => useContext(CardEngagementContext);

/**
 * Element handlers that feed one card's engagement conditions (a), (b) and (c) into the tracker.
 * Without a `cardId` (a row outside Needs you) the tracker is null and everything is inert.
 */
export function useEngagedCard(cardId: string | undefined) {
  const context = useEngagement();
  const tracker = cardId === undefined ? null : context;
  const id = cardId ?? "";
  const onFocusCapture = useCallback(() => tracker?.focusIn(id), [tracker, id]);
  const onBlurCapture = useCallback(
    (e: FocusEvent<HTMLElement>) => {
      if (!e.currentTarget.contains(e.relatedTarget as Node | null)) tracker?.focusOut(id);
    },
    [tracker, id],
  );
  const onPointerEnter = useCallback((e: PointerEvent) => e.pointerType !== "touch" && tracker?.pointer(id, true), [tracker, id]);
  const onPointerLeave = useCallback((e: PointerEvent) => e.pointerType !== "touch" && tracker?.pointer(id, false), [tracker, id]);
  const onTouchStart = useCallback((_e: TouchEvent) => tracker?.touch(id), [tracker, id]);

  const card = useMemo<CardEngagement>(
    () =>
      tracker
        ? {
            setSubStep: (v) => tracker.setSubStep(id, v),
            setHold: (v) => tracker.setHold(id, v),
            setResolved: (v) => tracker.setResolved(id, v),
          }
        : NOOP_CARD,
    [tracker, id],
  );
  const handlers = { onFocusCapture, onBlurCapture, onPointerEnter, onPointerLeave, onTouchStart };
  return { tracker, handlers, card };
}
