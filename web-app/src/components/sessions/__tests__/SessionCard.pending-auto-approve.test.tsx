/**
 * Tests for SessionCard's hasPendingAutoApproveChange predicate, which drives the
 * "Auto-approve pending" badge (shown when a Paused/Stopped session's auto_approve
 * value disagrees with whether the yolo flag is actually present in the last-launched
 * command). Mirrors SessionCard.pending-program.test.tsx's approach: unit-test the
 * exported predicate directly rather than rendering the full SessionCard.
 */

import { hasPendingAutoApproveChange } from "../SessionCard";
import type { Session } from "@/gen/session/v1/types_pb";
import { SessionStatus } from "@/gen/session/v1/types_pb";

function fakeSession(overrides: Partial<Session> = {}): Pick<Session, "status" | "autoApprove" | "launchCommand"> {
  return {
    status: SessionStatus.STOPPED,
    autoApprove: true,
    launchCommand: "claude --dangerously-skip-permissions",
    ...overrides,
  };
}

describe("hasPendingAutoApproveChange", () => {
  it("is true for a Stopped session where autoApprove is true but the flag isn't in the launch command yet", () => {
    const session = fakeSession({
      status: SessionStatus.STOPPED,
      autoApprove: true,
      launchCommand: "claude",
    });
    expect(hasPendingAutoApproveChange(session)).toBe(true);
  });

  it("is true for a Paused session where autoApprove is false but the flag is still in the launch command", () => {
    const session = fakeSession({
      status: SessionStatus.PAUSED,
      autoApprove: false,
      launchCommand: "aider --yes-always",
    });
    expect(hasPendingAutoApproveChange(session)).toBe(true);
  });

  it("is false when autoApprove and the launch command agree", () => {
    const session = fakeSession({
      status: SessionStatus.STOPPED,
      autoApprove: true,
      launchCommand: "claude --dangerously-skip-permissions",
    });
    expect(hasPendingAutoApproveChange(session)).toBe(false);
  });

  it("is false for an Active session even if autoApprove and the launch command disagree", () => {
    const session = fakeSession({
      status: SessionStatus.ACTIVE,
      autoApprove: true,
      launchCommand: "claude",
    });
    expect(hasPendingAutoApproveChange(session)).toBe(false);
  });

  it("is false when there is no launch command yet (never launched)", () => {
    const session = fakeSession({
      status: SessionStatus.STOPPED,
      autoApprove: true,
      launchCommand: "",
    });
    expect(hasPendingAutoApproveChange(session)).toBe(false);
  });
});
