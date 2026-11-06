import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { BottomNav } from "../BottomNav";

// Mock next/navigation
jest.mock("@/lib/nav", () =>
  require("@/lib/nav/testing-mock").navMockFromNext({
    navigation: () => ({
      usePathname: jest.fn(),
      useRouter: jest.fn(() => ({ push: jest.fn(), replace: jest.fn() })),
    }),
    link: () => {
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
    },
  }),
);

// Mock next/link so it renders as a plain anchor
// Mock ReviewQueueNavBadge to avoid context dependency
jest.mock("@/components/sessions/ReviewQueueNavBadge", () => ({
  ReviewQueueNavBadge: () => null,
}));

// Mock OmnibarContext
const mockOpenOmnibar = jest.fn();
jest.mock("@/lib/contexts/OmnibarContext", () => ({
  useOmnibar: () => ({ open: mockOpenOmnibar }),
}));

// Mock AuthContext
jest.mock("@/lib/contexts/AuthContext", () => ({
  useAuth: () => ({ authenticated: false, authEnabled: false }),
}));

// Mock NotificationContext
const mockTogglePanel = jest.fn();
let mockUnread = 0;
let mockNeedsAttention = 0;
jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({ togglePanel: mockTogglePanel, getUnreadCount: () => mockUnread }),
}));
jest.mock("@/lib/utils/notificationGrouping", () => ({
  countNeedsAttention: () => mockNeedsAttention,
}));

// Mock useHandedness so tests don't depend on localStorage or document.documentElement
jest.mock("@/lib/hooks/useHandedness", () => ({
  useHandedness: () => ({ leftHanded: false, toggleHandedness: jest.fn() }),
  HANDEDNESS_KEY: "stapler-squad:left-handed",
}));

// Mock FeatureFlagsContext
const mockFlags: Record<string, boolean> = {};
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlags: () => ({ flags: mockFlags }),
}));

// Mock the CSS module
jest.mock("../BottomNav.css", () => ({
  nav: "nav",
  navItem: "navItem",
  navItemActive: "navItemActive",
  navItemIcon: "navItemIcon",
  navItemLabel: "navItemLabel",
  newSessionButton: "newSessionButton",
  newSessionButtonInner: "newSessionButtonInner",
  notificationButton: "notificationButton",
  notificationIconWrap: "notificationIconWrap",
  notificationBadge: "notificationBadge",
  notificationBadgeAttention: "notificationBadgeAttention",
  moreBackdrop: "moreBackdrop",
  moreSheet: "moreSheet",
  moreSheetOpen: "moreSheetOpen",
  moreSheetItem: "moreSheetItem",
  moreSheetItemActive: "moreSheetItemActive",
  moreSheetItemIcon: "moreSheetItemIcon",
  moreSheetScrollable: "moreSheetScrollable",
  moreSheetSection: "moreSheetSection",
  moreSheetSectionHeader: "moreSheetSectionHeader",
  moreSheetUtilitySection: "moreSheetUtilitySection",
}));

import { usePathname } from "@/lib/nav";
import { BOTTOM_NAV_PRIMARY } from "@/lib/nav-pages";

const expectedPrimaryItems = BOTTOM_NAV_PRIMARY.filter((p) => !p.featureFlag);

describe("BottomNav", () => {
  beforeEach(() => {
    mockOpenOmnibar.mockClear();
    mockTogglePanel.mockClear();
    // JSDOM doesn't provide ResizeObserver; stub it so BottomNav's height measurement doesn't throw.
    global.ResizeObserver = jest.fn().mockImplementation(() => ({
      observe: jest.fn(),
      unobserve: jest.fn(),
      disconnect: jest.fn(),
    }));
  });

  it("renders all primary nav items and the New session button", () => {
    (usePathname as jest.Mock).mockReturnValue("/");
    render(<BottomNav />);

    for (const item of expectedPrimaryItems) {
      expect(screen.getByText(item.shortLabel ?? item.label)).toBeInTheDocument();
    }
    expect(screen.getByRole("button", { name: "Create new session" })).toBeInTheDocument();
  });

  it("opens omnibar when New session button is clicked", () => {
    (usePathname as jest.Mock).mockReturnValue("/");
    render(<BottomNav />);

    fireEvent.click(screen.getByRole("button", { name: "Create new session" }));
    expect(mockOpenOmnibar).toHaveBeenCalledTimes(1);
  });

  it("marks Sessions as active on home route", () => {
    (usePathname as jest.Mock).mockReturnValue("/");
    render(<BottomNav />);

    const sessionsLink = screen.getByText("Sessions").closest("a");
    expect(sessionsLink).toHaveAttribute("aria-current", "page");
  });

  it("does not mark Review as active on home route", () => {
    (usePathname as jest.Mock).mockReturnValue("/");
    render(<BottomNav />);

    const reviewLink = screen.getByText("Review").closest("a");
    expect(reviewLink).not.toHaveAttribute("aria-current", "page");
  });

  it("marks Review as active on review-queue route", () => {
    (usePathname as jest.Mock).mockReturnValue("/review-queue");
    render(<BottomNav />);

    const reviewLink = screen.getByText("Review").closest("a");
    expect(reviewLink).toHaveAttribute("aria-current", "page");
  });

  it("does not mark Sessions as active on review-queue route", () => {
    (usePathname as jest.Mock).mockReturnValue("/review-queue");
    render(<BottomNav />);

    const sessionsLink = screen.getByText("Sessions").closest("a");
    expect(sessionsLink).not.toHaveAttribute("aria-current", "page");
  });

  it("more_sheet_should_list_notifications_row_that_opens_the_tray", () => {
    render(<BottomNav />);
    const row = screen.getByTestId("more-notifications");
    expect(row).toHaveTextContent("Notifications (0)");
    fireEvent.click(row);
    expect(mockTogglePanel).toHaveBeenCalledTimes(1);
  });

  it("alerts_tab_should_open_the_tray_instead_of_navigating", () => {
    mockTogglePanel.mockClear();
    render(<BottomNav />);
    const alerts = screen.getByTestId("nav-alerts");
    expect(alerts.tagName).toBe("BUTTON");
    expect(alerts).toHaveAttribute("aria-controls", "notification-tray");
    fireEvent.click(alerts);
    expect(mockTogglePanel).toHaveBeenCalledTimes(1);
  });

  describe("alerts badge", () => {
    afterEach(() => {
      mockUnread = 0;
      mockNeedsAttention = 0;
    });

    it("shows no badge when nothing is unread", () => {
      render(<BottomNav />);
      expect(screen.getByTestId("nav-alerts")).toHaveAttribute("aria-label", "Notifications");
      expect(document.querySelector(".notificationBadge, .notificationBadgeAttention")).toBeNull();
    });

    it("uses the neutral badge and the exact count in the name when nothing needs you", () => {
      mockUnread = 150;
      render(<BottomNav />);
      const alerts = screen.getByTestId("nav-alerts");
      expect(alerts).toHaveAttribute("aria-label", "Notifications (150 unread)");
      expect(alerts.querySelector(".notificationBadge")).toHaveTextContent("99+");
      expect(alerts.querySelector(".notificationBadgeAttention")).toBeNull();
    });

    it("uses the attention badge and names the need-you count when something needs a decision", () => {
      mockUnread = 9;
      mockNeedsAttention = 2;
      render(<BottomNav />);
      const alerts = screen.getByTestId("nav-alerts");
      expect(alerts).toHaveAttribute("aria-label", "Notifications (9 unread, 2 need you)");
      expect(alerts.querySelector(".notificationBadgeAttention")).toHaveTextContent("9");
    });
  });

  it("renders the More button", () => {
    (usePathname as jest.Mock).mockReturnValue("/");
    render(<BottomNav />);

    expect(screen.getByRole("button", { name: "More navigation options" })).toBeInTheDocument();
  });
});

describe("BottomNav — ResizeObserver", () => {
  let resizeCallback: ResizeObserverCallback;
  const mockDisconnect = jest.fn();
  const mockObserve = jest.fn();
  let originalResizeObserver: typeof ResizeObserver;

  beforeAll(() => {
    originalResizeObserver = global.ResizeObserver;
  });

  afterAll(() => {
    global.ResizeObserver = originalResizeObserver;
  });

  beforeEach(() => {
    mockDisconnect.mockClear();
    mockObserve.mockClear();
    global.ResizeObserver = jest.fn().mockImplementation((cb: ResizeObserverCallback) => {
      resizeCallback = cb;
      return {
        observe: mockObserve,
        disconnect: mockDisconnect,
        unobserve: jest.fn(),
      };
    });
    // Reset CSS variable before each test
    document.documentElement.style.removeProperty("--bottom-nav-height");
    (usePathname as jest.Mock).mockReturnValue("/");
  });

  it("BottomNav_should_set_bottomNavHeight_When_mounted", () => {
    render(<BottomNav />);
    // observe must be called with the nav element (not null/undefined)
    expect(mockObserve).toHaveBeenCalledTimes(1);
    expect(mockObserve).toHaveBeenCalledWith(expect.any(HTMLElement));
    const observedElement = mockObserve.mock.calls[0][0] as HTMLElement;
    expect(observedElement.getAttribute("aria-label")).toBe("Bottom navigation");
    // --bottom-nav-height must be set to a px value (jsdom returns "0px" since no layout engine)
    const value = document.documentElement.style.getPropertyValue("--bottom-nav-height");
    expect(value).toMatch(/^\d+px$/);
  });

  it("BottomNav_should_update_bottomNavHeight_When_navResizes", () => {
    render(<BottomNav />);
    // Simulate ResizeObserver callback firing (e.g., nav height changed)
    act(() => {
      resizeCallback([], {} as ResizeObserver);
    });
    const value = document.documentElement.style.getPropertyValue("--bottom-nav-height");
    expect(value).toMatch(/^\d+px$/);
  });

  it("BottomNav_should_disconnect_ResizeObserver_When_unmounted", () => {
    const { unmount } = render(<BottomNav />);
    unmount();
    expect(mockDisconnect).toHaveBeenCalledTimes(1);
  });
});
