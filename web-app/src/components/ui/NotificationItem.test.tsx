/**
 * Tests for NotificationItem's resolved-approval badge and CI-block override actions.
 *
 * Task 2.3.2b/2.3.2d (Epic 2.3): a reconciled (rule-auto-resolved) approval renders a
 * distinct "Auto-resolved by rule: <name>" badge instead of the plain "✓ Approved"/
 * "✗ Denied" a live human decision gets, and the CI-block "Approve anyway" override
 * only renders when there is an actual CI-checks URL to approve against.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { NotificationItem } from "./NotificationItem";
import type { GroupedNotification } from "@/lib/utils/notificationGrouping";
import type { NotificationHistoryItem } from "@/lib/types/notification";

jest.mock("next/link", () => ({
  __esModule: true,
  default: ({
    href,
    children,
    ...props
  }: {
    href: string;
    children: React.ReactNode;
    [key: string]: unknown;
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));

function makeNotification(overrides: Partial<NotificationHistoryItem> = {}): NotificationHistoryItem {
  return {
    id: "notif-1",
    sessionId: "sess-1",
    sessionName: "My Session",
    message: "Wants to run a command",
    timestamp: Date.now(),
    notificationType: "approval_needed",
    isRead: false,
    metadata: {
      approval_id: "appr-1",
    },
    ...overrides,
  };
}

function makeGroup(notification: NotificationHistoryItem): GroupedNotification {
  return {
    notification,
    count: 1,
    allIds: [notification.id],
  };
}

function renderItem(props: Partial<React.ComponentProps<typeof NotificationItem>> & { group: GroupedNotification }) {
  return render(
    <NotificationItem
      resolvedApprovals={{}}
      pendingApprovals={{}}
      blockedApprovals={{}}
      failedApprovals={{}}
      resolveApproval={jest.fn()}
      removeFromHistory={jest.fn()}
      handleNotificationClick={jest.fn()}
      {...props}
    />
  );
}

describe("NotificationItem — resolved-approval badge", () => {
  it('shows the plain "✓ Approved" badge for a live (non-reconciled) allow decision', () => {
    const notification = makeNotification({ metadata: { approval_id: "appr-1" } });
    renderItem({
      group: makeGroup(notification),
      resolvedApprovals: { "appr-1": "allow" },
    });

    expect(screen.getByText("✓ Approved")).toBeInTheDocument();
    expect(screen.queryByText(/Auto-resolved by rule/)).not.toBeInTheDocument();
  });

  it('shows the plain "✗ Denied" badge for a live (non-reconciled) deny decision', () => {
    const notification = makeNotification({ metadata: { approval_id: "appr-1" } });
    renderItem({
      group: makeGroup(notification),
      resolvedApprovals: { "appr-1": "deny" },
    });

    expect(screen.getByText("✗ Denied")).toBeInTheDocument();
  });

  it('shows "Auto-resolved by rule: <name>" instead of "✓ Approved" when the notification is reconciled', () => {
    const notification = makeNotification({
      metadata: {
        approval_id: "appr-1",
        reconciled: "true",
        classifier_rule_name: "Auto-allow safe git status checks",
      },
    });
    renderItem({
      group: makeGroup(notification),
      resolvedApprovals: { "appr-1": "allow" },
    });

    expect(screen.queryByText("✓ Approved")).not.toBeInTheDocument();
    expect(screen.getByText(/Auto-resolved by rule: Auto-allow safe git status checks/)).toBeInTheDocument();
  });

  it('shows "Auto-resolved by rule: <name>" for a reconciled deny decision too', () => {
    const notification = makeNotification({
      metadata: {
        approval_id: "appr-1",
        reconciled: "true",
        classifier_rule_name: "Auto-deny risky commands",
      },
    });
    renderItem({
      group: makeGroup(notification),
      resolvedApprovals: { "appr-1": "deny" },
    });

    expect(screen.queryByText("✗ Denied")).not.toBeInTheDocument();
    expect(screen.getByText(/Auto-resolved by rule: Auto-deny risky commands/)).toBeInTheDocument();
  });

  it("falls back to a generic rule label when classifier_rule_name is absent on a reconciled notification", () => {
    const notification = makeNotification({
      metadata: { approval_id: "appr-1", reconciled: "true" },
    });
    renderItem({
      group: makeGroup(notification),
      resolvedApprovals: { "appr-1": "allow" },
    });

    expect(screen.getByText(/Auto-resolved by rule: a rule/)).toBeInTheDocument();
  });
});

describe("NotificationItem — CI-block override actions (Epic 2.3.2)", () => {
  it('renders "Approve anyway" and "View CI run" when the blocked message carries a CI-checks URL', () => {
    const notification = makeNotification({ metadata: { approval_id: "appr-1" } });
    renderItem({
      group: makeGroup(notification),
      blockedApprovals: {
        "appr-1": "Approval blocked: CI is failing on this branch — review before approving. https://github.com/org/repo/pull/1/checks",
      },
    });

    expect(screen.getByText("Approve anyway")).toBeInTheDocument();
    expect(screen.getByTestId("ci-block-view-run-link")).toBeInTheDocument();
    expect(screen.getByText("✗ Deny")).toBeInTheDocument();
  });

  it('omits "Approve anyway" (nothing left to approve against) when the blocked message has no CI-checks URL, e.g. a reconciliation race', () => {
    const notification = makeNotification({ metadata: { approval_id: "appr-1" } });
    renderItem({
      group: makeGroup(notification),
      blockedApprovals: {
        "appr-1": 'already auto-resolved by rule "Auto-allow safe git status checks" while you were reviewing it — no action needed',
      },
    });

    expect(screen.queryByText("Approve anyway")).not.toBeInTheDocument();
    expect(screen.queryByTestId("ci-block-view-run-link")).not.toBeInTheDocument();
    // Deny still applies — there's still a pending decision on the human's side of the race.
    expect(screen.getByText("✗ Deny")).toBeInTheDocument();
  });
});

describe("NotificationItem — transient resolveApproval failure (validation.md row 3)", () => {
  it("shows the retry message and keeps Approve/Deny enabled instead of collapsing to Expired", () => {
    const notification = makeNotification({ metadata: { approval_id: "appr-1" } });
    renderItem({
      group: makeGroup(notification),
      failedApprovals: { "appr-1": "Couldn't record your decision — try again." },
    });

    expect(screen.getByText("Couldn't record your decision — try again.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "✓ Approve" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "✗ Deny" })).toBeEnabled();
    expect(screen.queryByText("Expired")).not.toBeInTheDocument();
  });
});

describe("NotificationItem — optional removeFromHistory (Task 3.1.2b/3.1.5b)", () => {
  it("renders no ✕ control when removeFromHistory is omitted", () => {
    const notification = makeNotification();
    renderItem({ group: makeGroup(notification), removeFromHistory: undefined });
    expect(screen.queryByLabelText("Remove notification")).not.toBeInTheDocument();
  });

  it("renders the ✕ control and calls removeFromHistory when provided", () => {
    const removeFromHistory = jest.fn();
    const notification = makeNotification();
    renderItem({ group: makeGroup(notification), removeFromHistory });
    const button = screen.getByLabelText("Remove notification");
    expect(button).toBeInTheDocument();
    button.click();
    expect(removeFromHistory).toHaveBeenCalledWith("notif-1");
  });
});
