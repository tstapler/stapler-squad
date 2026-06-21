"use client";

import { SubStatus } from "@/gen/session/v1/types_pb";
import { assertNever } from "@/lib/utils/assertNever";
import {
  chipNeedsApproval,
  chipInputRequired,
  chipProcessing,
  chipError,
  chipTestsFailing,
  chipRateLimited,
  chipIdle,
  chipReady,
  chipSuccess,
  chipWaitingForAgent,
  spinner,
} from "./SubStatusChip.css";

interface SubStatusChipProps {
  subStatus: SubStatus;
}

/**
 * SubStatusChip renders a small inline chip showing fine-grained session activity.
 * Returns null for UNSPECIFIED only.
 *
 * Note: SessionRow filters out IDLE and READY before rendering this component —
 * those states are intentionally suppressed in the list view as low-signal noise.
 * Direct callers (e.g. detail headers) may still render IDLE/READY chips.
 */
export function SubStatusChip({ subStatus }: SubStatusChipProps) {
  switch (subStatus) {
    case SubStatus.WAITING_FOR_AGENT:
      return (
        <span
          className={chipWaitingForAgent}
          role="status"
          aria-label="Waiting for agents"
          title="Claude is waiting for background agents to finish"
        >
          ⏳ Waiting for Agents
        </span>
      );

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
          ⚠ Approve Tool Use
        </span>
      );

    case SubStatus.INPUT_REQUIRED:
      return (
        <span
          className={chipInputRequired}
          role="status"
          aria-label="Input needed"
          title="Waiting for you to type a response or select an option"
        >
          ⌨ Your Input Needed
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
      return (
        <span
          className={chipIdle}
          role="status"
          aria-label="Session is idle"
          title="Session is idle — waiting for your input"
        >
          ● Idle
        </span>
      );

    case SubStatus.READY:
      return (
        <span
          className={chipReady}
          role="status"
          aria-label="Ready for your next instruction"
          title="Session is at the prompt — ready for your next message"
        >
          ● Ready
        </span>
      );

    case SubStatus.SUCCESS:
      return (
        <span
          className={chipSuccess}
          role="status"
          aria-label="Task complete"
          title="Task completed successfully"
        >
          ✓ Done
        </span>
      );

    case SubStatus.UNSPECIFIED:
      return null;
    default:
      assertNever(subStatus);
      return null;
  }
}
