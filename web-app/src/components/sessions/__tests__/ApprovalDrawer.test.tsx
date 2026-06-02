/**
 * Tests for ApprovalDrawer component.
 *
 * Covers:
 *  - Returns null when isOpen=false
 *  - Renders drawer when isOpen=true
 *  - Shows empty state when no approvals
 *  - Shows approval count in heading
 *  - Approvals sorted by secondsRemaining ascending (most urgent first)
 *  - Escape key calls onClose
 *  - Close button calls onClose
 *  - aria-live region announces expiry when approval count decreases
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ApprovalDrawer } from "../ApprovalDrawer";
import type { PendingApprovalProto } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockApprove = jest.fn();
const mockDeny = jest.fn();
const mockRefresh = jest.fn();
let mockApprovals: PendingApprovalProto[] = [];

jest.mock("@/lib/hooks/useApprovals", () => ({
  useApprovals: () => ({
    approvals: mockApprovals,
    approve: mockApprove,
    deny: mockDeny,
    refresh: mockRefresh,
    clearForSession: jest.fn(),
    clearedSessions: new Set(),
  }),
}));

jest.mock("@/lib/store", () => ({
  useAppSelector: () => [],
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeApproval(
  id: string,
  secondsRemaining: number,
  sessionId = "session-1"
): PendingApprovalProto {
  return {
    id,
    sessionId,
    toolName: "Bash",
    toolInput: {},
    cwd: "/",
    secondsRemaining,
  } as unknown as PendingApprovalProto;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("ApprovalDrawer — visibility", () => {
  it("renders nothing when isOpen=false", () => {
    mockApprovals = [];
    const { container } = render(
      <ApprovalDrawer isOpen={false} onClose={jest.fn()} />
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("renders the drawer when isOpen=true", () => {
    mockApprovals = [];
    render(<ApprovalDrawer isOpen={true} onClose={jest.fn()} />);
    expect(screen.getByRole("complementary", { name: /pending approvals/i })).toBeInTheDocument();
  });
});

describe("ApprovalDrawer — empty state", () => {
  it("shows 'No pending approvals' when list is empty", () => {
    mockApprovals = [];
    render(<ApprovalDrawer isOpen={true} onClose={jest.fn()} />);
    expect(screen.getByText("No pending approvals")).toBeInTheDocument();
  });

  it("does not show count in heading when empty", () => {
    mockApprovals = [];
    render(<ApprovalDrawer isOpen={true} onClose={jest.fn()} />);
    expect(screen.getByRole("heading")).toHaveTextContent("Pending Approvals");
    expect(screen.getByRole("heading").textContent).not.toMatch(/\(\d/);
  });
});

describe("ApprovalDrawer — approval count", () => {
  it("shows count in heading when approvals exist", () => {
    mockApprovals = [makeApproval("a1", 30), makeApproval("a2", 60)];
    render(<ApprovalDrawer isOpen={true} onClose={jest.fn()} />);
    expect(screen.getByRole("heading")).toHaveTextContent("(2)");
  });
});

describe("ApprovalDrawer — sort order", () => {
  it("renders most urgent approval (lowest secondsRemaining) first", () => {
    mockApprovals = [
      makeApproval("slow", 120),
      makeApproval("urgent", 5),
      makeApproval("medium", 60),
    ];
    render(<ApprovalDrawer isOpen={true} onClose={jest.fn()} />);
    const cards = document.querySelectorAll("[data-testid^='approval-card-']");
    expect(cards[0].getAttribute("data-testid")).toBe("approval-card-urgent");
    expect(cards[1].getAttribute("data-testid")).toBe("approval-card-medium");
    expect(cards[2].getAttribute("data-testid")).toBe("approval-card-slow");
  });
});

describe("ApprovalDrawer — keyboard and close", () => {
  it("calls onClose when Escape is pressed", () => {
    mockApprovals = [];
    const onClose = jest.fn();
    render(<ApprovalDrawer isOpen={true} onClose={onClose} />);
    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("calls onClose when close button is clicked", () => {
    mockApprovals = [];
    const onClose = jest.fn();
    render(<ApprovalDrawer isOpen={true} onClose={onClose} />);
    fireEvent.click(screen.getByRole("button", { name: /close approvals drawer/i }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

describe("ApprovalDrawer — aria-live announcements", () => {
  it("announces when approval count decreases", async () => {
    mockApprovals = [makeApproval("a1", 30), makeApproval("a2", 60)];
    const { rerender } = render(<ApprovalDrawer isOpen={true} onClose={jest.fn()} />);

    // Simulate one approval resolving/expiring
    mockApprovals = [makeApproval("a2", 60)];
    rerender(<ApprovalDrawer isOpen={true} onClose={jest.fn()} />);

    await waitFor(() => {
      const announcer = document.querySelector('[aria-live="polite"]');
      expect(announcer?.textContent).toMatch(/expired or resolved/i);
    });
  });
});
