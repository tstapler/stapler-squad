import React from "react";
import { render, screen } from "@testing-library/react";
import { DrawerNav } from "../DrawerNav";

// Mock next/navigation — writable so individual tests can override usePathname
const mockUsePathname = jest.fn(() => "/");
jest.mock("next/navigation", () => ({
  usePathname: () => mockUsePathname(),
}));

// Mock next/link
jest.mock("next/link", () => ({
  __esModule: true,
  default: ({
    href,
    children,
    className,
    ...props
  }: {
    href: string;
    children: React.ReactNode;
    className?: string;
    [key: string]: unknown;
  }) => (
    <a href={href} className={className} {...props}>
      {children}
    </a>
  ),
}));

// Mock NavigationContext
jest.mock("@/lib/contexts/NavigationContext", () => ({
  useNavigation: () => ({ isDrawerOpen: true, toggleDrawer: jest.fn() }),
}));

// Mock badge components
jest.mock("@/components/sessions/ReviewQueueNavBadge", () => ({
  ReviewQueueNavBadge: () => <span data-testid="review-badge" />,
}));
jest.mock("@/components/unfinished/UnfinishedNavBadge", () => ({
  UnfinishedNavBadge: () => <span data-testid="unfinished-badge" />,
}));
jest.mock("@/components/backlog-stuck/StuckNavBadge", () => ({
  StuckNavBadge: () => <span data-testid="stuck-badge" />,
}));
jest.mock("@/components/ui/NotificationsNavBadge", () => ({
  NotificationsNavBadge: () => <span data-testid="notifications-badge" />,
}));

// Mock FeatureFlagsContext — default: no flags
const mockUseFeatureFlags = jest.fn(() => ({ flags: {} }));
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlags: () => mockUseFeatureFlags(),
}));

// Mock DrawerNav.css — return class name strings
jest.mock("../DrawerNav.css", () => ({
  drawer: jest.fn(() => "drawer"),
  navList: "navList",
  navItem: jest.fn(() => "navItem"),
  navIcon: "navIcon",
  navLabel: jest.fn(() => "navLabel"),
  sectionHeader: jest.fn(() => "sectionHeader"),
  sectionSpacer: "sectionSpacer",
  navBadgeWrapper: jest.fn(() => "navBadgeWrapper"),
  toggleButton: "toggleButton",
  drawerDivider: "drawerDivider",
}));

describe("DrawerNav", () => {
  beforeEach(() => {
    mockUseFeatureFlags.mockReturnValue({ flags: {} });
    mockUsePathname.mockReturnValue("/");
  });

  it("DrawerNav_should_renderAllFourGroupHeaders_When_drawerIsOpen", () => {
    render(<DrawerNav />);
    // Group headers are rendered in aria-hidden <li> elements; use getAllByText to handle
    // pages that share a name with their group (e.g., "Insights" page inside "Insights" group).
    expect(screen.getAllByText("Work").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("Automation").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("Insights").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("Settings & Tools").length).toBeGreaterThanOrEqual(1);
  });

  it("DrawerNav_should_renderSessionsInWorkGroup_When_drawerIsOpen", () => {
    render(<DrawerNav />);
    expect(screen.getByText("Sessions")).toBeInTheDocument();
  });

  it("DrawerNav_should_filterFeatureFlaggedItem_When_flagIsOff", () => {
    mockUseFeatureFlags.mockReturnValue({ flags: {} });
    render(<DrawerNav />);
    expect(screen.queryByText("Backlog")).not.toBeInTheDocument();
  });

  it("DrawerNav_should_showFeatureFlaggedItem_When_flagIsOn", () => {
    mockUseFeatureFlags.mockReturnValue({ flags: { backlog: true } });
    render(<DrawerNav />);
    expect(screen.getByText("Backlog")).toBeInTheDocument();
  });

  it("DrawerNav_should_markActiveItem_When_pathnameMatches", () => {
    mockUsePathname.mockReturnValue("/review-queue");
    render(<DrawerNav />);
    const reviewLink = screen.getByText("Review Queue").closest("a");
    expect(reviewLink).toHaveAttribute("aria-current", "page");
  });
});
