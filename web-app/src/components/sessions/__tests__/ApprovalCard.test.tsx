/**
 * Tests for ApprovalCard component.
 *
 * Covers:
 *  - Shows Approve and Deny buttons when secondsRemaining > 0
 *  - Shows only Dismiss button when secondsRemaining <= 0 (expired)
 *  - Dismiss calls onDeny
 *  - Approve calls onApprove
 *  - Deny calls onDeny
 *  - Expired card has cardExpired CSS class
 *  - Session title shown when provided
 *  - Falls back to sessionId when title not provided
 *  - Tool input preview shown for known input fields
 */

import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { ApprovalCard } from "../ApprovalCard";
import type { PlainApproval } from "@/lib/api/approvalsApi";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeApproval(overrides: Partial<PlainApproval> = {}): PlainApproval {
  return {
    id: "approval-1",
    sessionId: "session-abc",
    toolName: "Bash",
    toolInput: {},
    cwd: "/home/user",
    permissionMode: "default",
    createdAt: undefined,
    expiresAt: undefined,
    secondsRemaining: 60,
    riskLevel: "",
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Action buttons based on expiry state
// ---------------------------------------------------------------------------

describe("ApprovalCard — action buttons", () => {
  it("shows Approve and Deny when not expired", () => {
    const approval = makeApproval({ secondsRemaining: 60 });
    render(<ApprovalCard approval={approval} onApprove={jest.fn()} onDeny={jest.fn()} />);
    expect(screen.getByRole("button", { name: /approve/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /deny/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /dismiss/i })).not.toBeInTheDocument();
  });

  it("shows only Dismiss when expired (secondsRemaining = 0)", () => {
    const approval = makeApproval({ secondsRemaining: 0 });
    render(<ApprovalCard approval={approval} onApprove={jest.fn()} onDeny={jest.fn()} />);
    expect(screen.getByRole("button", { name: /dismiss/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /approve/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /deny/i })).not.toBeInTheDocument();
  });

  it("shows only Dismiss when secondsRemaining is negative", () => {
    const approval = makeApproval({ secondsRemaining: -5 });
    render(<ApprovalCard approval={approval} onApprove={jest.fn()} onDeny={jest.fn()} />);
    expect(screen.getByRole("button", { name: /dismiss/i })).toBeInTheDocument();
  });

  it("Approve button calls onApprove", () => {
    const onApprove = jest.fn();
    const approval = makeApproval({ secondsRemaining: 30 });
    render(<ApprovalCard approval={approval} onApprove={onApprove} onDeny={jest.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /approve/i }));
    expect(onApprove).toHaveBeenCalledTimes(1);
  });

  it("Deny button calls onDeny", () => {
    const onDeny = jest.fn();
    const approval = makeApproval({ secondsRemaining: 30 });
    render(<ApprovalCard approval={approval} onApprove={jest.fn()} onDeny={onDeny} />);
    fireEvent.click(screen.getByRole("button", { name: /deny/i }));
    expect(onDeny).toHaveBeenCalledTimes(1);
  });

  it("Dismiss button calls onDeny", () => {
    const onDeny = jest.fn();
    const approval = makeApproval({ secondsRemaining: 0 });
    render(<ApprovalCard approval={approval} onApprove={jest.fn()} onDeny={onDeny} />);
    fireEvent.click(screen.getByRole("button", { name: /dismiss/i }));
    expect(onDeny).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// Expiry CSS class
// ---------------------------------------------------------------------------

describe("ApprovalCard — expiry state styling", () => {
  it("card does not have cardExpired class when not expired", () => {
    const approval = makeApproval({ secondsRemaining: 60 });
    const { container } = render(
      <ApprovalCard approval={approval} onApprove={jest.fn()} onDeny={jest.fn()} />
    );
    // identity-obj-proxy returns class name as "cardExpired"
    const card = container.firstChild as HTMLElement;
    expect(card.className).not.toMatch(/cardExpired/);
  });

  it("card has cardExpired class when expired", () => {
    const approval = makeApproval({ secondsRemaining: 0 });
    const { container } = render(
      <ApprovalCard approval={approval} onApprove={jest.fn()} onDeny={jest.fn()} />
    );
    const card = container.firstChild as HTMLElement;
    expect(card.className).toMatch(/cardExpired/);
  });
});

// ---------------------------------------------------------------------------
// Session title display
// ---------------------------------------------------------------------------

describe("ApprovalCard — session title", () => {
  it("shows sessionTitle when provided", () => {
    const approval = makeApproval({ sessionId: "raw-id" });
    render(
      <ApprovalCard
        approval={approval}
        onApprove={jest.fn()}
        onDeny={jest.fn()}
        sessionTitle="My Feature Branch"
      />
    );
    expect(screen.getByText("My Feature Branch")).toBeInTheDocument();
  });

  it("shows sessionId when title not provided", () => {
    const approval = makeApproval({ sessionId: "raw-id", secondsRemaining: 60 });
    render(<ApprovalCard approval={approval} onApprove={jest.fn()} onDeny={jest.fn()} />);
    expect(screen.getByText("raw-id")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Tool input preview
// ---------------------------------------------------------------------------

describe("ApprovalCard — tool input preview", () => {
  it("shows command preview when toolInput has command field", () => {
    const approval = makeApproval({
      toolInput: { command: "npm test" },
      secondsRemaining: 60,
    });
    render(<ApprovalCard approval={approval} onApprove={jest.fn()} onDeny={jest.fn()} />);
    expect(screen.getByText("npm test")).toBeInTheDocument();
  });

  it("shows file_path preview when toolInput has file_path field", () => {
    const approval = makeApproval({
      toolInput: { file_path: "/src/index.ts" },
      secondsRemaining: 60,
    });
    render(<ApprovalCard approval={approval} onApprove={jest.fn()} onDeny={jest.fn()} />);
    expect(screen.getByText("/src/index.ts")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Severity badge (review-queue-severity Epic 5, Story 5.1)
// ---------------------------------------------------------------------------

describe("ApprovalCard — severity badge", () => {
  it("ApprovalCard_should_RenderSeverityBadgeInHeader_When_ApprovalHasRiskLevel", () => {
    const approval = makeApproval({ riskLevel: "critical" });
    render(<ApprovalCard approval={approval} onApprove={jest.fn()} onDeny={jest.fn()} />);
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "Critical risk");
  });

  it("renders the not-recorded state when riskLevel is empty", () => {
    const approval = makeApproval({ riskLevel: "" });
    render(<ApprovalCard approval={approval} onApprove={jest.fn()} onDeny={jest.fn()} />);
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "Severity not recorded");
  });
});
