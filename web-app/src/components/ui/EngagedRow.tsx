"use client";

import { useEffect, useRef, useState, type HTMLAttributes, type KeyboardEvent, type MouseEvent, type ReactNode } from "react";
import { CardEngagementContext, useEngagedCard } from "@/lib/hooks/useEngagedCard";

const INERT_TARGET = "input, textarea, select, [contenteditable], .xterm";
const INTERACTIVE = "button, a, input, textarea, select, [role=button], [role=menuitem]";
const DIGIT = /^[1-9]$/;
const TOUCH_CLICK_MS = 1_000;

interface Props extends HTMLAttributes<HTMLDivElement> {
  cardId: string;
  /** The first Needs-you item on panel open: expanded by the program, never engaged or armed by that. */
  preExpand?: boolean;
  children: ReactNode;
}

const hasActions = (el: HTMLElement) => !!el.querySelector('[data-testid="action-row"]');

/**
 * A Needs-you card as the engagement tracker sees it (design/ux.md 7.1): reports the operator's
 * focus, pointer and touch, toggles expansion on Enter, Space or a tap, collapses on Esc, and
 * moves focus (never sends) to the Nth option on a digit while the card is digit-armed.
 */
export function EngagedRow({ cardId, preExpand = false, children, onKeyDown, ...rest }: Props) {
  const { tracker, handlers, card } = useEngagedCard(cardId);
  const [expansion, setExpansion] = useState(() => tracker?.expansion(cardId) ?? "none");
  const lastTouchAt = useRef(Number.NEGATIVE_INFINITY);

  useEffect(() => {
    if (!tracker || !preExpand) return;
    tracker.preExpand(cardId);
    setExpansion(tracker.expansion(cardId));
  }, [tracker, cardId, preExpand]);

  useEffect(() => () => tracker?.forget(cardId), [tracker, cardId]);

  const toggle = () => tracker && setExpansion(tracker.toggleExpanded(cardId));

  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    onKeyDown?.(e);
    if (!tracker || e.defaultPrevented) return;
    const target = e.target as HTMLElement;
    if (target.closest(INERT_TARGET) || e.nativeEvent.isComposing || e.ctrlKey || e.metaKey || e.altKey) return;
    if ((e.key === "Enter" || e.key === " ") && target === e.currentTarget && hasActions(e.currentTarget)) {
      e.preventDefault();
      e.stopPropagation();
      toggle();
    } else if (e.key === "Escape" && tracker.expansion(cardId) !== "none") {
      e.preventDefault();
      e.stopPropagation();
      tracker.collapse(cardId);
      setExpansion("none");
    } else if (DIGIT.test(e.key) && tracker.isDigitArmed(cardId)) {
      const options = e.currentTarget.querySelectorAll<HTMLElement>("[data-action-option]");
      const nth = options[Number(e.key) - 1];
      if (nth) {
        e.preventDefault();
        tracker.focusByDigit(nth);
      }
    }
  };

  const onClick = (e: MouseEvent<HTMLDivElement>) => {
    rest.onClick?.(e);
    const touched = Date.now() - lastTouchAt.current < TOUCH_CLICK_MS;
    if (touched && !(e.target as HTMLElement).closest(INTERACTIVE) && hasActions(e.currentTarget)) toggle();
  };

  return (
    <CardEngagementContext.Provider value={card}>
      <div
        {...rest}
        {...handlers}
        onTouchStart={(e) => {
          lastTouchAt.current = Date.now();
          handlers.onTouchStart(e);
        }}
        onKeyDown={onKey}
        onClick={onClick}
        data-engaged-card={cardId}
        data-expanded={expansion !== "none" ? expansion : undefined}
      >
        {children}
      </div>
    </CardEngagementContext.Provider>
  );
}
