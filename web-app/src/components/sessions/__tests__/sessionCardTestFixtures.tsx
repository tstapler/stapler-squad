/**
 * Shared jest.mock factory bodies for SessionCard's heavy dependency mocks.
 *
 * SessionCard.click.test.tsx, SessionCard.approval-suppression.test.tsx,
 * SessionCard.dedup-integration.test.tsx, and ../SessionCard.test.tsx all mock
 * the same set of SessionCard dependencies identically. babel-jest hoists
 * `jest.mock(...)` calls above imports, so each spec file keeps its own thin
 * `jest.mock("module", () => require("./sessionCardTestFixtures").mockXyz())`
 * line — only the factory bodies live here, deduplicated.
 */

import React from "react";

export function mockConnect() {
  return { createClient: jest.fn(() => ({})) };
}

export function mockConnectWeb() {
  return { createConnectTransport: jest.fn(() => ({ unary: jest.fn(), stream: jest.fn() })) };
}

export function mockReviewQueueContext() {
  return { useReviewQueueContext: () => ({ items: [] }) };
}

export function mockSessionServiceContext() {
  return {
    useSessionServiceContext: () => ({
      draftPullRequest: jest.fn(),
      createPullRequest: jest.fn(),
    }),
  };
}

export function mockStore() {
  return { useAppSelector: jest.fn(() => ({})) };
}

export function mockSessionsSlice() {
  return { selectDetectedStatusMap: jest.fn() };
}

export function mockUseTerminalSnapshot() {
  return { useTerminalSnapshot: () => ({ snapshot: null, loading: false }) };
}

export function mockUseFocusTrap() {
  return { useFocusTrap: () => {} };
}

export function mockAppLink() {
  return {
    AppLink: ({ href, children, ...rest }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
      <a href={href} {...rest}>{children}</a>
    ),
  };
}

export function mockModal() {
  return {
    Modal: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    ModalContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    ModalTitle: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    ModalFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  };
}

// Radix's real Tooltip only reveals its label on hover after a delay via a portal —
// mock it so the label is assertable synchronously without simulating pointer timing.
export function mockTooltip() {
  return {
    Tooltip: ({ children, label }: { children: React.ReactNode; label: string }) => (
      <div data-testid="tooltip-mock" data-label={label}>{children}</div>
    ),
  };
}

export function mockUseSessionActions() {
  return {
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
  };
}
