/**
 * Tests for SessionCard's always-visible pause-reason badge — promoted from
 * tooltip-only (unreachable on touch/mobile) to a plain-text span rendered in
 * normal flow, so the reason is readable without hovering on any viewport.
 *
 * Covers:
 *  - SessionCard_should_RenderVisiblePauseReasonText_When_SessionIsPausedWithReason
 *  - SessionCard_should_RenderNoPauseText_When_SessionIsNotPaused
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { SessionCard } from "../SessionCard";
import { SessionStatus } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// Heavy dependency mocks (mirrors SessionCard.click.test.tsx)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Fixture
// ---------------------------------------------------------------------------

const minimalSession: Partial<Session> = {
  id: "s1",
  title: "Test Session",
  tags: [],
  category: "",
  path: "/tmp/session",
  branch: "",
  program: "claude",
};

describe("SessionCard — pause reason", () => {
  it("SessionCard_should_RenderVisiblePauseReasonText_When_SessionIsPausedWithReason", () => {
    const session = {
      ...minimalSession,
      status: SessionStatus.PAUSED,
      pauseReason: "auto:inactivity",
    } as unknown as Session;
    render(<SessionCard session={session} />);
    expect(screen.getByText("Paused automatically — no recent activity")).toBeInTheDocument();
  });

  it("SessionCard_should_RenderNoPauseText_When_SessionIsNotPaused", () => {
    const session = {
      ...minimalSession,
      status: SessionStatus.ACTIVE,
      pauseReason: "",
    } as unknown as Session;
    render(<SessionCard session={session} />);
    expect(screen.queryByText(/Paused/)).not.toBeInTheDocument();
  });
});
