/**
 * Tests for SessionCard's hasPendingProgramChange predicate, which drives the
 * "pending program change" badge (shown when a Paused/Stopped session's program was
 * changed but the session hasn't been relaunched with it yet).
 *
 * Unit-tests the exported predicate directly rather than rendering the full
 * SessionCard, which requires a large, currently-incomplete Redux/analytics mocking
 * setup unrelated to this feature (see the sibling test files' existing mock lists).
 *
 * Covers:
 *  - SessionCard_should_showPendingProgramBadge_When_stoppedSessionProgramChangedSinceLastLaunch
 */

import { hasPendingProgramChange } from "../SessionCard";
import type { Session } from "@/gen/session/v1/types_pb";
import { SessionStatus } from "@/gen/session/v1/types_pb";

function fakeSession(overrides: Partial<Session> = {}): Pick<Session, "status" | "program" | "launchCommand"> {
  return {
    status: SessionStatus.STOPPED,
    program: "claude",
    launchCommand: "claude --dangerously-skip-permissions",
    ...overrides,
  };
}

describe("hasPendingProgramChange", () => {
  it("is true for a Stopped session whose program changed since the last launch", () => {
    const session = fakeSession({
      status: SessionStatus.STOPPED,
      program: "aider",
      launchCommand: "claude --dangerously-skip-permissions",
    });
    expect(hasPendingProgramChange(session)).toBe(true);
  });

  it("is true for a Paused session whose program changed since the last launch", () => {
    const session = fakeSession({
      status: SessionStatus.PAUSED,
      program: "aider",
      launchCommand: "claude --dangerously-skip-permissions",
    });
    expect(hasPendingProgramChange(session)).toBe(true);
  });

  it("is false when the launch command already matches the current program", () => {
    const session = fakeSession({
      status: SessionStatus.STOPPED,
      program: "claude",
      launchCommand: "claude --dangerously-skip-permissions",
    });
    expect(hasPendingProgramChange(session)).toBe(false);
  });

  it("is false for an Active session even if the program changed", () => {
    const session = fakeSession({
      status: SessionStatus.ACTIVE,
      program: "aider",
      launchCommand: "claude --dangerously-skip-permissions",
    });
    expect(hasPendingProgramChange(session)).toBe(false);
  });

  it("is false when there is no launch command yet (never launched)", () => {
    const session = fakeSession({
      status: SessionStatus.STOPPED,
      program: "aider",
      launchCommand: "",
    });
    expect(hasPendingProgramChange(session)).toBe(false);
  });
});
