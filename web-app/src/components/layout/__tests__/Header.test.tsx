import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { Header } from "../Header";

// Mock next/navigation
jest.mock("next/navigation", () => ({
  usePathname: jest.fn(),
  useRouter: jest.fn(() => ({ push: jest.fn(), replace: jest.fn() })),
}));

// Mock next/link as a plain anchor so href is directly testable
jest.mock("next/link", () => {
  const MockLink = React.forwardRef<
    HTMLAnchorElement,
    React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string; prefetch?: boolean }
  >(function MockLink({ href, children, prefetch: _prefetch, ...rest }, ref) {
    return (
      <a href={href} ref={ref} {...rest}>
        {children}
      </a>
    );
  });
  MockLink.displayName = "MockLink";
  return MockLink;
});

jest.mock("@/components/sessions/ReviewQueueNavBadge", () => ({
  ReviewQueueNavBadge: () => null,
}));
jest.mock("@/components/sessions/ApprovalNavBadge", () => ({
  ApprovalNavBadge: () => null,
}));
jest.mock("@/components/unfinished/UnfinishedNavBadge", () => ({
  UnfinishedNavBadge: () => null,
}));
jest.mock("@/components/backlog-stuck/StuckNavBadge", () => ({
  StuckNavBadge: () => null,
}));
jest.mock("@/components/ui/DebugMenu", () => ({
  DebugMenu: () => null,
}));
jest.mock("@/components/layout/WorkspaceSwitcher", () => ({
  WorkspaceSwitcher: () => null,
}));
jest.mock("@/components/layout/BottomNav", () => ({
  BottomNav: () => null,
}));
jest.mock("@/components/layout/ConnectionIndicator", () => ({
  ConnectionIndicator: () => null,
}));
jest.mock("@/components/sessions/ApprovalDrawer", () => ({
  ApprovalDrawer: () => null,
}));
jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({ togglePanel: jest.fn(), getUnreadCount: () => 0 }),
}));
jest.mock("@/lib/contexts/OmnibarContext", () => ({
  useOmnibar: () => ({ open: jest.fn() }),
}));
jest.mock("@/lib/contexts/AuthContext", () => ({
  useAuth: () => ({ authenticated: false, authEnabled: false }),
}));

import { usePathname } from "next/navigation";

describe("Header nav links", () => {
  beforeEach(() => {
    jest.spyOn(window.history, "replaceState");
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("Unfinished link points to /unfinished", () => {
    (usePathname as jest.Mock).mockReturnValue("/");
    render(<Header />);

    const link = screen.getByRole("link", { name: /unfinished/i });
    expect(link).toHaveAttribute("href", "/unfinished");
  });

  it("Review Queue link points to /review-queue", () => {
    (usePathname as jest.Mock).mockReturnValue("/");
    render(<Header />);

    const link = screen.getByRole("link", { name: /review queue/i });
    expect(link).toHaveAttribute("href", "/review-queue");
  });

  it("does not call window.history.replaceState when clicking Unfinished", () => {
    // Regression: replaceState was being called directly from the nav click handler,
    // which Next.js intercepts and turns into a "/" navigation, cancelling Link's target route.
    (usePathname as jest.Mock).mockReturnValue("/");
    render(<Header />);

    const link = screen.getByRole("link", { name: /unfinished/i });
    fireEvent.click(link);

    expect(window.history.replaceState).not.toHaveBeenCalled();
  });

  it("does not call window.history.replaceState when clicking Review Queue", () => {
    (usePathname as jest.Mock).mockReturnValue("/");
    render(<Header />);

    const link = screen.getByRole("link", { name: /review queue/i });
    fireEvent.click(link);

    expect(window.history.replaceState).not.toHaveBeenCalled();
  });

  it("marks Sessions as active on home route", () => {
    (usePathname as jest.Mock).mockReturnValue("/");
    render(<Header />);

    const link = screen.getByRole("link", { name: /^sessions$/i });
    expect(link).toHaveAttribute("aria-current", "page");
  });

  it("marks Unfinished as active on /unfinished route", () => {
    (usePathname as jest.Mock).mockReturnValue("/unfinished");
    render(<Header />);

    const link = screen.getByRole("link", { name: /unfinished/i });
    expect(link).toHaveAttribute("aria-current", "page");
  });

  it("marks Review Queue as active on /review-queue route", () => {
    (usePathname as jest.Mock).mockReturnValue("/review-queue");
    render(<Header />);

    const link = screen.getByRole("link", { name: /review queue/i });
    expect(link).toHaveAttribute("aria-current", "page");
  });

  it("does not mark Sessions as active on /unfinished route", () => {
    (usePathname as jest.Mock).mockReturnValue("/unfinished");
    render(<Header />);

    const link = screen.getByRole("link", { name: /^sessions$/i });
    expect(link).not.toHaveAttribute("aria-current", "page");
  });
});
