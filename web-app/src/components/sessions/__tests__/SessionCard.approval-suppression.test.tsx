/**
 * Tests for SessionCard suppressApprovalSubStatus prop.
 *
 * Covers:
 *  - T-UNIT-TS-018: Hides SubStatusChip when suppressApprovalSubStatus=true + NEEDS_APPROVAL
 *  - T-UNIT-TS-019: Shows non-approval subStatus even when suppressApprovalSubStatus=true
 *  - T-UNIT-TS-020: Hides "Needs Approval" StatusBadge when suppressApprovalSubStatus=true
 *  - T-UNIT-TS-021: Shows other detectedStatus even when suppressApprovalSubStatus=true
 *  - T-UNIT-TS-022: Shows both badges normally when suppressApprovalSubStatus=false (default)
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { SessionCard } from "../SessionCard";
import type { Session } from "@/gen/session/v1/types_pb";
import { SessionStatus, SubStatus } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// Mocks
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

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: "s1",
    title: "Test Session",
    status: SessionStatus.ACTIVE,
    subStatus: SubStatus.UNSPECIFIED,
    tags: [],
    category: "",
    path: "/tmp/session",
    branch: "",
    program: "claude",
    ...overrides,
  } as unknown as Session;
}

// renderCard helper — kept for future tests; uses makeSession() as default session
function renderCard(props: Partial<React.ComponentProps<typeof SessionCard>> = {}) {
  const { session, ...rest } = props;
  return render(
    <SessionCard
      session={session ?? makeSession()}
      {...rest}
    />
  );
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("SessionCard — suppressApprovalSubStatus prop", () => {
  it("T-UNIT-TS-018: SessionCard_should_hideNeedsApprovalBadge_When_suppressApprovalSubStatusTrue", () => {
    const session = makeSession({ subStatus: SubStatus.NEEDS_APPROVAL });
    render(
      <SessionCard
        session={session}
        suppressApprovalSubStatus={true}
      />
    );

    // SubStatusChip for NEEDS_APPROVAL has aria-label "Needs approval"
    expect(screen.queryByRole("status", { name: /needs approval/i })).not.toBeInTheDocument();
  });

  it("T-UNIT-TS-019: SessionCard_should_showOtherSubStatus_When_suppressApprovalSubStatusTrue", () => {
    const session = makeSession({ subStatus: SubStatus.PROCESSING });
    render(
      <SessionCard
        session={session}
        suppressApprovalSubStatus={true}
      />
    );

    // PROCESSING chip should still be visible — only NEEDS_APPROVAL is suppressed
    expect(screen.getByRole("status", { name: /processing/i })).toBeInTheDocument();
  });

  it("T-UNIT-TS-020: SessionCard_should_hideNeedsApprovalStatusBadge_When_suppressApprovalSubStatusTrue", () => {
    const session = makeSession({ subStatus: SubStatus.UNSPECIFIED });
    render(
      <SessionCard
        session={session}
        detectedStatus="Needs Approval"
        suppressApprovalSubStatus={true}
      />
    );

    // "Needs Approval" detectedStatus badge should be suppressed
    expect(screen.queryByText(/Needs Approval/i)).not.toBeInTheDocument();
  });

  it("T-UNIT-TS-021: SessionCard_should_showOtherDetectedStatus_When_suppressApprovalSubStatusTrue", () => {
    const session = makeSession({ subStatus: SubStatus.UNSPECIFIED });
    render(
      <SessionCard
        session={session}
        detectedStatus="Running Tests"
        suppressApprovalSubStatus={true}
      />
    );

    // Non-approval detectedStatus should still be shown
    expect(screen.getByText("Running Tests")).toBeInTheDocument();
  });

  it("T-UNIT-TS-022: SessionCard_should_showNeedsApprovalBadge_When_suppressApprovalSubStatusFalse", () => {
    const session = makeSession({ subStatus: SubStatus.NEEDS_APPROVAL });
    render(
      <SessionCard
        session={session}
        suppressApprovalSubStatus={false}
      />
    );

    // SubStatusChip for NEEDS_APPROVAL should appear normally
    expect(screen.getByRole("status", { name: /needs approval/i })).toBeInTheDocument();
  });

  it("SessionCard_should_showNeedsApprovalBadge_When_suppressApprovalSubStatusOmitted", () => {
    const session = makeSession({ subStatus: SubStatus.NEEDS_APPROVAL });
    render(
      <SessionCard
        session={session}
        // suppressApprovalSubStatus not passed — defaults to false
      />
    );

    expect(screen.getByRole("status", { name: /needs approval/i })).toBeInTheDocument();
  });
});
