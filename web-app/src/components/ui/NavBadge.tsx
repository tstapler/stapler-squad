// +feature: nav-badge
"use client";

import { badge, inline as inlineClass, empty } from "./NavBadge.css";

interface NavBadgeBaseProps {
  /** The count to display in the badge. When 0 and showWhenEmpty is false, renders null. */
  count: number;
  /** Whether to display the badge inline (adds left margin and vertically centres it). */
  inline?: boolean;
  /** When true, renders the badge even when count is 0. Default: false. */
  showWhenEmpty?: boolean;
}

interface NavBadgeButtonProps
  extends NavBadgeBaseProps,
    Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, "children"> {
  element: "button";
}

interface NavBadgeSpanProps
  extends NavBadgeBaseProps,
    Omit<React.HTMLAttributes<HTMLSpanElement>, "children"> {
  element: "span";
}

export type NavBadgeProps = NavBadgeButtonProps | NavBadgeSpanProps;

function buildClassName(inline: boolean, isEmpty: boolean): string {
  return [badge, inline && inlineClass, isEmpty && empty].filter(Boolean).join(" ");
}

/**
 * Primitive badge component for navigation count indicators.
 *
 * Renders either a `<button>` (for clickable badges) or a `<span>` (for
 * display-only badges). ApprovalNavBadge, ReviewQueueNavBadge, and
 * NotificationsNavBadge all delegate their visual rendering to this primitive.
 */
export function NavBadge(props: NavBadgeProps) {
  const { count, element, inline = false, showWhenEmpty = false, ...rest } = props;

  if (count === 0 && !showWhenEmpty) return null;

  const className = buildClassName(inline, count === 0);
  const displayValue = count > 99 ? "99+" : count;

  if (element === "button") {
    return (
      <button
        {...(rest as React.ButtonHTMLAttributes<HTMLButtonElement>)}
        className={className}
      >
        {displayValue}
      </button>
    );
  }

  return (
    <span
      {...(rest as React.HTMLAttributes<HTMLSpanElement>)}
      className={className}
    >
      {displayValue}
    </span>
  );
}
