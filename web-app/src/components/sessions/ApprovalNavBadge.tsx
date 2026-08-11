"use client";

import { useApprovalsContext } from "@/lib/contexts/ApprovalsContext";
import { NavBadge } from "@/components/ui/NavBadge";

interface ApprovalNavBadgeProps {
  inline?: boolean;
  onClick?: () => void;
}

/**
 * Navigation badge that displays the count of pending tool-use approvals.
 * Used in the header navigation to indicate approvals awaiting user decision.
 *
 * Hidden when there are no pending approvals.
 */
export function ApprovalNavBadge({ inline = false, onClick }: ApprovalNavBadgeProps) {
  const { approvals } = useApprovalsContext();

  const count = approvals.length;

  return (
    <NavBadge
      count={count}
      element="button"
      inline={inline}
      data-testid="approval-nav-badge"
      aria-label={`${count} pending approval${count !== 1 ? "s" : ""}. Click to review.`}
      title={`${count} tool-use request${count !== 1 ? "s" : ""} awaiting approval`}
      onClick={onClick}
    />
  );
}
