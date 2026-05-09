/**
 * Tests for SessionCard click propagation behavior (Bug 1 fix).
 *
 * Covers:
 *  - TC-1.1: Normal click stops propagation (outerClick not called)
 *  - TC-1.2: Normal click calls onClick prop
 *  - TC-1.3: Alt+click stops propagation and calls onOpenInNewPane
 *  - TC-1.4: Select mode click stops propagation (regression guard)
 */

import React from "react";
import { render, fireEvent } from "@testing-library/react";
import { SessionCard } from "../SessionCard";
import type { Session } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// Heavy dependency mocks
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
// Minimal session fixture
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("SessionCard — stopPropagation", () => {
  it("SessionCard_should_stopPropagation_When_normalClick", () => {
    const outerClick = jest.fn();
    const onSessionClick = jest.fn();
    const { getByTestId } = render(
      <div onClick={outerClick}>
        <SessionCard
          session={minimalSession as Session}
          onClick={onSessionClick}
        />
      </div>
    );
    fireEvent.click(getByTestId("session-card"));
    expect(onSessionClick).toHaveBeenCalledTimes(1);
    expect(outerClick).not.toHaveBeenCalled(); // propagation stopped
  });

  it("SessionCard_should_callOnClick_When_normalClick", () => {
    const onSessionClick = jest.fn();
    const { getByTestId } = render(
      <SessionCard
        session={minimalSession as Session}
        onClick={onSessionClick}
      />
    );
    fireEvent.click(getByTestId("session-card"));
    expect(onSessionClick).toHaveBeenCalledTimes(1);
  });

  it("SessionCard_should_stopPropagation_When_altClick", () => {
    const outerClick = jest.fn();
    const onOpenInNewPane = jest.fn();
    const { getByTestId } = render(
      <div onClick={outerClick}>
        <SessionCard
          session={minimalSession as Session}
          onOpenInNewPane={onOpenInNewPane}
        />
      </div>
    );
    fireEvent.click(getByTestId("session-card"), { altKey: true });
    expect(onOpenInNewPane).toHaveBeenCalledTimes(1);
    expect(outerClick).not.toHaveBeenCalled();
  });

  it("SessionCard_should_stopPropagation_When_selectMode", () => {
    const outerClick = jest.fn();
    const onToggleSelect = jest.fn();
    const { getByTestId } = render(
      <div onClick={outerClick}>
        <SessionCard
          session={minimalSession as Session}
          selectMode={true}
          onToggleSelect={onToggleSelect}
        />
      </div>
    );
    fireEvent.click(getByTestId("session-card"));
    expect(onToggleSelect).toHaveBeenCalledTimes(1);
    expect(outerClick).not.toHaveBeenCalled();
  });
});
