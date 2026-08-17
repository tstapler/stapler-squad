import React from "react";
import { render, screen } from "@testing-library/react";
import { SessionCard } from "./SessionCard";
import { SessionStatus } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";
import { staleBadge } from "./SessionCard.css";

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({})),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({ unary: jest.fn(), stream: jest.fn() })),
}));

jest.mock("@/lib/contexts/ReviewQueueContext", () => ({
  useReviewQueueContext: () => ({ items: [] }),
}));

jest.mock("@/lib/store", () => ({
  useAppSelector: jest.fn(() => ({})),
}));

jest.mock("@/lib/store/sessionsSlice", () => ({
  selectDetectedStatusMap: jest.fn(),
}));

jest.mock("@/lib/hooks/useTerminalSnapshot", () => ({
  useTerminalSnapshot: () => ({ snapshot: null, loading: false }),
}));

jest.mock("@/lib/hooks/useFocusTrap", () => ({
  useFocusTrap: () => {},
}));

jest.mock("@/components/ui/AppLink", () => ({
  AppLink: ({ href, children, ...rest }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
    <a href={href} {...rest}>{children}</a>
  ),
}));

jest.mock("@/components/ui/Modal", () => ({
  Modal: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalTitle: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

// Radix's real Tooltip only reveals its label on hover after a delay via a portal —
// mock it so the label is assertable synchronously without simulating pointer timing.
jest.mock("@/components/ui/Tooltip", () => ({
  Tooltip: ({ children, label }: { children: React.ReactNode; label: string }) => (
    <div data-testid="tooltip-mock" data-label={label}>{children}</div>
  ),
}));

jest.mock("@/lib/hooks/useSessionActions", () => ({
  useSessionActions: () => ({
    pause: jest.fn(),
    resume: jest.fn(),
    delete: jest.fn(),
    rename: jest.fn(),
    restart: jest.fn(),
    createCheckpoint: jest.fn(),
    updateTags: jest.fn(),
    update: jest.fn(),
  }),
}));

const minimalSession: Partial<Session> = {
  id: "s1",
  title: "Test Session",
  status: 1 as Session["status"],
  tags: [],
  category: "",
  path: "/tmp/session",
  branch: "",
  program: "claude",
};

describe("SessionCard — note badge", () => {
  it("SessionCard_should_RenderNoteBadge_When_SessionNoteIsNonEmpty", () => {
    const session = { ...minimalSession, note: "waiting on CI" } as unknown as Session;
    render(<SessionCard session={session} />);
    expect(screen.getByTestId("badge-has-note")).toBeInTheDocument();
  });

  it("SessionCard_should_NotRenderNoteBadge_When_SessionNoteIsWhitespaceOrEmpty", () => {
    const whitespaceSession = { ...minimalSession, note: "   \n" } as unknown as Session;
    const { rerender } = render(<SessionCard session={whitespaceSession} />);
    expect(screen.queryByTestId("badge-has-note")).toBeNull();

    const emptySession = { ...minimalSession, note: "" } as unknown as Session;
    rerender(<SessionCard session={emptySession} />);
    expect(screen.queryByTestId("badge-has-note")).toBeNull();
  });

  it("SessionCard_should_TruncateNoteBadgeTooltipToOneHundredTwentyChars_When_NoteIsLong", () => {
    const longNote = "N".repeat(150);
    const session = { ...minimalSession, note: longNote } as unknown as Session;
    render(<SessionCard session={session} />);
    const badge = screen.getByTestId("badge-has-note");
    const expected = "N".repeat(119) + "…";
    expect(badge.closest('[data-testid="tooltip-mock"]')).toHaveAttribute("data-label", expected);
  });
});

// Builds a Timestamp-shaped object (seconds/nanos) the number of minutes ago from now —
// matches the {seconds: bigint, nanos: number} shape session-staleness.ts and SessionCard's
// own formatTimeAgo/formatDate helpers read from lastMeaningfulOutput/lastTerminalUpdate.
function minutesAgoTimestamp(minutes: number) {
  const seconds = Math.floor(Date.now() / 1000) - minutes * 60;
  return { seconds: BigInt(seconds), nanos: 0 };
}

describe("SessionCard — stale badge", () => {
  it("SessionCard_should_RenderStaleBadge_When_ActiveSessionExceedsThreshold", () => {
    const session = {
      ...minimalSession,
      status: SessionStatus.ACTIVE,
      lastMeaningfulOutput: minutesAgoTimestamp(45),
    } as unknown as Session;
    render(<SessionCard session={session} staleThresholdMinutes={30} />);

    const badge = screen.getByText("Stale", { exact: false });
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveAttribute("role", "img");
    expect(badge.getAttribute("aria-label")).toMatch(/^Stale — no output for/);
  });

  it("SessionCard_should_NotRenderStaleBadge_When_PausedSessionLastOutputWasSixHoursAgo", () => {
    const session = {
      ...minimalSession,
      status: SessionStatus.PAUSED,
      lastMeaningfulOutput: minutesAgoTimestamp(6 * 60),
    } as unknown as Session;
    render(<SessionCard session={session} staleThresholdMinutes={30} />);

    expect(screen.queryByText("Stale", { exact: false })).toBeNull();
  });

  it("SessionCard_should_ApplyStaleBadgeWarningTokenStyle_When_StaleBadgeRenders", () => {
    const session = {
      ...minimalSession,
      status: SessionStatus.ACTIVE,
      lastMeaningfulOutput: minutesAgoTimestamp(45),
    } as unknown as Session;
    render(<SessionCard session={session} staleThresholdMinutes={30} />);

    const badge = screen.getByText("Stale", { exact: false });
    // Reuses the same warning-token-based style exported for the badge — asserts the
    // rendered class matches the `staleBadge` export (same toHaveClass(String(...))
    // pattern ReviewQueuePanel.test.tsx uses for its own vanilla-extract class assertions,
    // and the same warning tokens stuckReason.css.ts's chipStaleWork applies via
    // vars.color.warningBg/warningText/warning).
    expect(badge).toHaveClass(String(staleBadge));
  });
});
