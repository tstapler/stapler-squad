/**
 * Tests for the pi approval-extension health badge (pi-support Epic 4.2,
 * Story 4.2.2). Covers:
 *  - piHealthBadgeInfo's exact icon/label/aria-label per state (unit-tests the
 *    exported pure function directly, mirroring SessionCard.pending-program.test.tsx's
 *    "test the predicate, not the full render" convention).
 *  - The badge is shown only when pi-support is on AND program is "pi", and never
 *    renders as "loaded" before a signal arrives (defaults to Unknown).
 */

import "./sessionCardMockSetup";
import React from "react";
import { render, screen } from "@testing-library/react";
import { piHealthBadgeInfo, SessionCard } from "../SessionCard";
import { PiExtensionHealth, SessionStatus } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// piHealthBadgeInfo unit tests
// ---------------------------------------------------------------------------

describe("piHealthBadgeInfo", () => {
  it("Loaded: exact aria-label text", () => {
    expect(piHealthBadgeInfo(PiExtensionHealth.LOADED).ariaLabel).toBe(
      "pi approval extension: loaded — tool calls are enforced"
    );
  });

  it("Failed: exact aria-label text, naming the unenforced-tool-calls consequence", () => {
    expect(piHealthBadgeInfo(PiExtensionHealth.FAILED).ariaLabel).toBe(
      "pi approval extension: not loaded — tool calls are unenforced"
    );
  });

  it("Unknown: exact aria-label text", () => {
    expect(piHealthBadgeInfo(PiExtensionHealth.UNKNOWN).ariaLabel).toBe(
      "pi approval extension: status unknown"
    );
  });

  it("Unspecified (wire default, pre-signal) reads identically to Unknown, never Loaded", () => {
    const unspecified = piHealthBadgeInfo(PiExtensionHealth.UNSPECIFIED);
    const unknown = piHealthBadgeInfo(PiExtensionHealth.UNKNOWN);
    expect(unspecified.ariaLabel).toBe(unknown.ariaLabel);
    expect(unspecified.ariaLabel).not.toContain("loaded — tool calls are enforced");
  });

  it("each state has a distinct label/icon combination, independent of color (AC3)", () => {
    const loaded = piHealthBadgeInfo(PiExtensionHealth.LOADED);
    const failed = piHealthBadgeInfo(PiExtensionHealth.FAILED);
    const unknown = piHealthBadgeInfo(PiExtensionHealth.UNKNOWN);
    // All three carry the literal "pi" label plus a distinguishing icon glyph.
    for (const state of [loaded, failed, unknown]) {
      expect(state.label).toBe("pi");
      expect(state.icon.length).toBeGreaterThan(0);
    }
    expect(new Set([loaded.icon, failed.icon, unknown.icon]).size).toBe(3);
    expect(new Set([loaded.className, failed.className, unknown.className]).size).toBe(3);
  });
});

// ---------------------------------------------------------------------------
// Full-render visibility gating (flag on/off, program pi/non-pi)
// ---------------------------------------------------------------------------

let piSupportEnabled = false;
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: (name: string) => (name === "pi-support" ? piSupportEnabled : false),
}));

function fakeSession(overrides: Partial<Session> = {}): Partial<Session> {
  return {
    id: "s1",
    title: "pi session",
    status: SessionStatus.ACTIVE,
    program: "pi",
    tags: [],
    piExtensionHealth: PiExtensionHealth.UNSPECIFIED,
    ...overrides,
  };
}

describe("SessionCard pi health badge visibility", () => {
  beforeEach(() => {
    piSupportEnabled = false;
  });

  it("does not render when pi-support flag is off, even for a pi session", () => {
    piSupportEnabled = false;
    render(<SessionCard session={fakeSession() as Session} />);
    expect(screen.queryByTestId("pi-health-badge")).toBeNull();
  });

  it("does not render for a non-pi program even when pi-support is on", () => {
    piSupportEnabled = true;
    render(<SessionCard session={fakeSession({ program: "claude" }) as Session} />);
    expect(screen.queryByTestId("pi-health-badge")).toBeNull();
  });

  it("renders with the Unknown state on first paint, never Loaded, before any signal arrives", () => {
    piSupportEnabled = true;
    render(<SessionCard session={fakeSession({ piExtensionHealth: PiExtensionHealth.UNSPECIFIED }) as Session} />);
    const badge = screen.getByTestId("pi-health-badge");
    expect(badge.getAttribute("aria-label")).toBe("pi approval extension: status unknown");
  });

  it("renders the Loaded state's aria-label once the server reports it", () => {
    piSupportEnabled = true;
    render(<SessionCard session={fakeSession({ piExtensionHealth: PiExtensionHealth.LOADED }) as Session} />);
    const badge = screen.getByTestId("pi-health-badge");
    expect(badge.getAttribute("aria-label")).toBe("pi approval extension: loaded — tool calls are enforced");
  });

  it("renders the Failed state's aria-label, and it is keyboard/AT-discoverable without hover (AC6)", () => {
    piSupportEnabled = true;
    render(<SessionCard session={fakeSession({ piExtensionHealth: PiExtensionHealth.FAILED }) as Session} />);
    const badge = screen.getByRole("img", { name: /pi approval extension/ });
    expect(badge.getAttribute("aria-label")).toBe("pi approval extension: not loaded — tool calls are unenforced");
  });
});
