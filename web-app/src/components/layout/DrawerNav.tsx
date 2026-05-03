"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useNavigation } from "@/lib/contexts/NavigationContext";
import {
  drawer,
  navList,
  navItem,
  navIcon,
  navLabel,
  badge,
  toggleButton,
  drawerDivider,
} from "./DrawerNav.css";

interface NavEntry {
  href: string;
  icon: string;
  label: string;
  badgeCount?: number;
}

const NAV_ITEMS: NavEntry[] = [
  { href: "/", icon: "▣", label: "Sessions" },
  { href: "/review-queue", icon: "⚠", label: "Review Queue" },
  { href: "/history", icon: "◷", label: "History" },
  { href: "/rules", icon: "⊡", label: "Rules" },
  { href: "/config", icon: "⚙", label: "Config" },
  { href: "/logs", icon: "≡", label: "Logs" },
];

interface DrawerNavProps {
  /** Optional badge counts injected from outside (e.g., approval count) */
  reviewQueueCount?: number;
  sessionCount?: number;
}

export function DrawerNav({ reviewQueueCount, sessionCount }: DrawerNavProps) {
  const { isDrawerOpen, toggleDrawer } = useNavigation();
  const pathname = usePathname();

  const itemsWithBadges = NAV_ITEMS.map((item) => {
    if (item.href === "/" && sessionCount !== undefined) {
      return { ...item, badgeCount: sessionCount };
    }
    if (item.href === "/review-queue" && reviewQueueCount !== undefined) {
      return { ...item, badgeCount: reviewQueueCount };
    }
    return item;
  });

  return (
    <nav
      className={drawer({ open: isDrawerOpen })}
      data-testid="drawer-nav"
      aria-label="Main navigation"
    >
      <ul className={navList} role="list">
        {itemsWithBadges.map((item) => {
          const isActive =
            item.href === "/"
              ? pathname === "/"
              : pathname.startsWith(item.href);

          return (
            <li key={item.href}>
              <Link
                href={item.href}
                className={navItem({ active: isActive })}
                aria-current={isActive ? "page" : undefined}
                title={!isDrawerOpen ? item.label : undefined}
              >
                <span className={navIcon} aria-hidden="true">
                  {item.icon}
                </span>
                <span className={navLabel({ visible: isDrawerOpen })}>
                  {item.label}
                </span>
                {item.badgeCount !== undefined && item.badgeCount > 0 && (
                  <span className={badge} aria-label={`${item.badgeCount} items`}>
                    {item.badgeCount > 99 ? "99+" : item.badgeCount}
                  </span>
                )}
              </Link>
            </li>
          );
        })}
      </ul>

      <div className={drawerDivider} />

      <button
        className={toggleButton}
        onClick={toggleDrawer}
        aria-label="Toggle navigation"
        aria-expanded={isDrawerOpen}
        title={isDrawerOpen ? "Collapse navigation" : "Expand navigation"}
      >
        <span aria-hidden="true">{isDrawerOpen ? "◀" : "▶"}</span>
      </button>
    </nav>
  );
}
