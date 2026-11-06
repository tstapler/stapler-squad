/**
 * Characterises how the five pathname consumers react to `/review-queue` vs `/review-queue/`.
 *
 * Findings under Next today (what these tests assert):
 *  - Header, DrawerNav and BottomNav use `startsWith`, so the trailing slash does not change them.
 *  - Navigation (`pathname === item.href`) and CockpitShell (`pathname !== "/review-queue"`) are
 *    exact comparisons, so the slash case is wrong. Those two cases are `it.failing` until Story 2.1
 *    normalises `usePathname()`; remove `.failing` then.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { Header } from "../Header";
import { DrawerNav } from "../DrawerNav";
import { BottomNav } from "../BottomNav";
import { CockpitShell } from "../CockpitShell";
import { Navigation } from "@/components/ui/Navigation";

let mockPathname = "/";

jest.mock("@/lib/nav", () =>
  require("@/lib/nav/testing-mock").navMockFromNext({
    navigation: () => ({
      usePathname: () => mockPathname,
      useRouter: () => ({ push: jest.fn(), replace: jest.fn() }),
    }),
    link: () => ({
      __esModule: true,
      default: ({ href, children, prefetch: _prefetch, ...rest }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string; prefetch?: boolean }) => (
        <a href={href} {...rest}>{children}</a>
      ),
    }),
  }),
);
jest.mock("@/components/sessions/ReviewQueueNavBadge", () => ({ ReviewQueueNavBadge: () => null }));
jest.mock("@/components/sessions/ApprovalNavBadge", () => ({ ApprovalNavBadge: () => null }));
jest.mock("@/components/sessions/MemoryNavBadge", () => ({ MemoryNavBadge: () => null }));
jest.mock("@/components/unfinished/UnfinishedNavBadge", () => ({ UnfinishedNavBadge: () => null }));
jest.mock("@/components/backlog-stuck/StuckNavBadge", () => ({ StuckNavBadge: () => null }));
jest.mock("@/components/ui/NotificationsNavBadge", () => ({ NotificationsNavBadge: () => null }));
jest.mock("@/components/ui/DebugMenu", () => ({ DebugMenu: () => null }));
jest.mock("@/components/layout/WorkspaceSwitcher", () => ({ WorkspaceSwitcher: () => null }));
jest.mock("@/components/layout/ConnectionIndicator", () => ({ ConnectionIndicator: () => null }));
jest.mock("@/components/changelog/UpdateBadge", () => ({ UpdateBadge: () => null }));
jest.mock("@/components/sessions/ApprovalDrawer", () => ({ ApprovalDrawer: () => null }));
jest.mock("@/components/command-bar/CommandBar", () => ({ CommandBar: () => null }));
jest.mock("@/components/ui/KeyboardShortcutOverlay", () => ({ KeyboardShortcutOverlay: () => null }));
jest.mock("@/lib/shortcuts/useShortcut", () => ({ useShortcut: jest.fn() }));
jest.mock("@/lib/shortcuts/useNotificationShortcuts", () => ({ useNotificationShortcuts: jest.fn() }));
jest.mock("@/components/providers/ViewportProvider", () => ({
  useViewport: () => ({ isMobile: false, isFoldable: false }),
}));
jest.mock("@/lib/contexts/NavigationContext", () => ({
  useNavigation: () => ({ isDrawerOpen: true, toggleDrawer: jest.fn() }),
}));
jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({ togglePanel: jest.fn(), getUnreadCount: () => 0, isPanelOpen: false }),
}));
jest.mock("@/lib/utils/notificationGrouping", () => ({ countNeedsAttention: () => 0 }));
jest.mock("@/lib/contexts/OmnibarContext", () => ({ useOmnibar: () => ({ open: jest.fn() }) }));
jest.mock("@/lib/contexts/AuthContext", () => ({
  useAuth: () => ({ authenticated: false, authEnabled: false }),
}));
jest.mock("@/lib/hooks/useHandedness", () => ({
  useHandedness: () => ({ leftHanded: false, toggleHandedness: jest.fn() }),
  HANDEDNESS_KEY: "stapler-squad:left-handed",
}));
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlags: () => ({ flags: {} }),
  useFeatureFlag: () => false,
}));

const REVIEW_QUEUE = "/review-queue";
const SHORTCUTS_BUTTON = "Keyboard shortcuts (?)";

function activeHrefs(container: HTMLElement): string[] {
  // Header embeds BottomNav, so the same href can appear twice.
  const hrefs = Array.from(container.querySelectorAll('a[aria-current="page"]')).map((a) => a.getAttribute("href") ?? "");
  return Array.from(new Set(hrefs));
}

beforeEach(() => {
  global.ResizeObserver = jest.fn().mockImplementation(() => ({
    observe: jest.fn(),
    unobserve: jest.fn(),
    disconnect: jest.fn(),
  }));
});

const startsWithConsumers: Array<[string, () => React.ReactElement]> = [
  ["Header", () => <Header />],
  ["DrawerNav", () => <DrawerNav />],
  ["BottomNav", () => <BottomNav />],
];

describe.each(startsWithConsumers)("%s active state", (_name, element) => {
  it.each([REVIEW_QUEUE, `${REVIEW_QUEUE}/`])("marks only the review queue link current for %s", (pathname) => {
    mockPathname = pathname;
    const { container } = render(element());
    expect(activeHrefs(container)).toEqual([REVIEW_QUEUE]);
  });
});

describe("Navigation active state", () => {
  it("marks the review queue link current for /review-queue", () => {
    mockPathname = REVIEW_QUEUE;
    const { container } = render(<Navigation />);
    expect(activeHrefs(container)).toEqual([REVIEW_QUEUE]);
  });

  // Story 2.1: remove .failing once usePathname() is normalised.
  it.failing("marks the review queue link current for /review-queue/", () => {
    mockPathname = `${REVIEW_QUEUE}/`;
    const { container } = render(<Navigation />);
    expect(activeHrefs(container)).toEqual([REVIEW_QUEUE]);
  });
});

describe("CockpitShell floating shortcuts button", () => {
  it("is hidden on /review-queue", () => {
    mockPathname = REVIEW_QUEUE;
    render(<CockpitShell>x</CockpitShell>);
    expect(screen.queryByRole("button", { name: SHORTCUTS_BUTTON })).toBeNull();
  });

  it("is shown elsewhere", () => {
    mockPathname = "/history";
    render(<CockpitShell>x</CockpitShell>);
    expect(screen.getByRole("button", { name: SHORTCUTS_BUTTON })).toBeInTheDocument();
  });

  // Story 2.1: remove .failing once usePathname() is normalised.
  it.failing("is hidden on /review-queue/ too", () => {
    mockPathname = `${REVIEW_QUEUE}/`;
    render(<CockpitShell>x</CockpitShell>);
    expect(screen.queryByRole("button", { name: SHORTCUTS_BUTTON })).toBeNull();
  });
});
