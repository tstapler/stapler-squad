"use client";

import { SubStatus } from "@/gen/session/v1/types_pb";
import {
  chipNeedsApproval,
  chipProcessing,
  chipError,
  chipTestsFailing,
  chipRateLimited,
  spinner,
} from "./SubStatusChip.css";

interface SubStatusChipProps {
  subStatus: SubStatus;
}

/**
 * SubStatusChip renders a small inline chip showing fine-grained session activity.
 * Returns null for UNSPECIFIED and IDLE — those states do not need a visible indicator.
 * Only intended for sessions with lifecycle status ACTIVE.
 */
export function SubStatusChip({ subStatus }: SubStatusChipProps) {
  switch (subStatus) {
    case SubStatus.PROCESSING:
      return (
        <span
          className={chipProcessing}
          role="status"
          aria-label="Session is processing"
          title="Claude is actively working"
        >
          <span className={spinner} aria-hidden="true" />
          Thinking…
        </span>
      );

    case SubStatus.NEEDS_APPROVAL:
      return (
        <span
          className={chipNeedsApproval}
          role="status"
          aria-label="Needs approval"
          title="Waiting for your approval on a tool request"
        >
          🔔 Needs Approval
        </span>
      );

    case SubStatus.ERROR:
      return (
        <span
          className={chipError}
          role="status"
          aria-label="Error"
          title="Session encountered an error"
        >
          ✖ Error
        </span>
      );

    case SubStatus.TESTS_FAILING:
      return (
        <span
          className={chipTestsFailing}
          role="status"
          aria-label="Tests failing"
          title="Tests are currently failing"
        >
          ⚠ Tests Failing
        </span>
      );

    case SubStatus.RATE_LIMITED:
      return (
        <span
          className={chipRateLimited}
          role="status"
          aria-label="Rate limited"
          title="Session is experiencing API rate limiting"
        >
          ⏱ Rate Limited
        </span>
      );

    case SubStatus.IDLE:
    case SubStatus.UNSPECIFIED:
    default:
      return null;
  }
}
