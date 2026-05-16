"use client";
// +feature: ui:bottom-nav

import { useState, useEffect, useRef } from "react";
import type { LucideIcon } from "lucide-react";
import {
  LayoutGrid,
  Clock4,
  ClipboardCheck,
  History,
  ScrollText,
  BookOpen,
  SlidersHorizontal,
  Settings,
  Bell,
  Plus,
  MoreHorizontal,
  User,
} from "lucide-react";
import { AppLink } from "@/components/ui/AppLink";
import { usePathname } from "next/navigation";
import { ReviewQueueNavBadge } from "@/components/sessions/ReviewQueueNavBadge";
import { useOmnibar } from "@/lib/contexts/OmnibarContext";
import { useAuth } from "@/lib/contexts/AuthContext";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { routes } from "@/lib/routes";
import * as styles from "./BottomNav.css";

type BottomNavItem = { href: string; label: string; icon: LucideIcon };

const primaryItems: BottomNavItem[] = [
  { href: routes.home, label: "Sessions", icon: LayoutGrid },
  { href: routes.unfinished, label: "Unfinished", icon: Clock4 },
  { href: routes.reviewQueue, label: "Review", icon: ClipboardCheck },
];

const moreItems: BottomNavItem[] = [
  { href: routes.history, label: "History", icon: History },
  { href: routes.logs, label: "Logs", icon: ScrollText },
  { href: routes.rules, label: "Rules", icon: BookOpen },
  { href: routes.config, label: "Config", icon: SlidersHorizontal },
  { href: routes.settings, label: "Settings", icon: Settings },
];

type PrimaryItem = BottomNavItem;
type MoreItem = BottomNavItem;

export function BottomNav() {
  const pathname = usePathname();
  const { open: openOmnibar } = useOmnibar();
  const { authenticated, authEnabled } = useAuth();
  const { getUnreadCount } = useNotifications();
  const unreadCount = getUnreadCount();
  const [moreOpen, setMoreOpen] = useState(false);

  // Close the more menu on route change
  useEffect(() => {
    setMoreOpen(false);
  }, [pathname]);

  // Close on Escape
  useEffect(() => {
    if (!moreOpen) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") setMoreOpen(false);
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [moreOpen]);

  // Measure actual nav height (includes safe-area padding) and publish to CSS
  const navRef = useRef<HTMLElement>(null);
  useEffect(() => {
    const nav = navRef.current;
    if (!nav) return;
    const update = () => {
      document.documentElement.style.setProperty(
        "--bottom-nav-height",
        `${nav.offsetHeight}px`
      );
    };
    const ro = new ResizeObserver(update);
    ro.observe(nav);
    update(); // set immediately on mount
    return () => ro.disconnect();
  }, []);

  const isMoreActive = moreItems.some((item) => pathname?.startsWith(item.href));

  const renderPrimaryItem = (item: PrimaryItem) => {
    const isActive =
      item.href === routes.home
        ? pathname === routes.home
        : pathname?.startsWith(item.href);
    const Icon = item.icon;

    return (
      <AppLink
        key={item.href}
        href={item.href}
        className={`${styles.navItem} ${isActive ? styles.navItemActive : ""}`}
        aria-current={isActive ? "page" : undefined}
      >
        <span className={styles.navItemIcon} aria-hidden="true">
          {item.label === "Review" ? (
            <>
              <Icon size={20} />
              <ReviewQueueNavBadge inline={true} />
            </>
          ) : (
            <Icon size={20} />
          )}
        </span>
        <span className={styles.navItemLabel}>{item.label}</span>
      </AppLink>
    );
  };

  return (
    <>
      {/* Backdrop */}
      {moreOpen && (
        <div
          className={styles.moreBackdrop}
          onClick={() => setMoreOpen(false)}
          aria-hidden="true"
        />
      )}

      {/* More sheet */}
      <div
        className={`${styles.moreSheet} ${moreOpen ? styles.moreSheetOpen : ""}`}
        aria-label="More navigation"
        role="navigation"
      >
        {moreItems.map((item: MoreItem) => {
          const isActive = pathname?.startsWith(item.href);
          const Icon = item.icon;
          return (
            <AppLink
              key={item.href}
              href={item.href}
              className={`${styles.moreSheetItem} ${isActive ? styles.moreSheetItemActive : ""}`}
              aria-current={isActive ? "page" : undefined}
            >
              <span className={styles.moreSheetItemIcon} aria-hidden="true"><Icon size={20} /></span>
              <span>{item.label}</span>
            </AppLink>
          );
        })}
        {authEnabled && authenticated && (
          <AppLink
            href={routes.account}
            className={`${styles.moreSheetItem} ${pathname === routes.account ? styles.moreSheetItemActive : ""}`}
            aria-current={pathname === routes.account ? "page" : undefined}
          >
            <span className={styles.moreSheetItemIcon} aria-hidden="true"><User size={20} /></span>
            <span>Account</span>
          </AppLink>
        )}
      </div>

      {/* Bottom nav bar */}
      <nav ref={navRef} className={styles.nav} aria-label="Bottom navigation">
        {primaryItems.map(renderPrimaryItem)}
        <AppLink
          href={routes.notifications}
          className={`${styles.navItem} ${styles.notificationButton} ${pathname === routes.notifications ? styles.navItemActive : ""}`}
          aria-current={pathname === routes.notifications ? "page" : undefined}
          aria-label={unreadCount > 0 ? `Notifications (${unreadCount} unread)` : "Notifications"}
        >
          <span className={styles.notificationIconWrap} aria-hidden="true">
            <Bell size={20} />
            {unreadCount > 0 && (
              <span className={styles.notificationBadge} aria-label={`${unreadCount} unread`}>
                {unreadCount > 9 ? "9+" : unreadCount}
              </span>
            )}
          </span>
          <span className={styles.navItemLabel}>Alerts</span>
        </AppLink>
        <button
          className={styles.newSessionButton}
          onClick={openOmnibar}
          aria-label="Create new session"
        >
          <span className={styles.newSessionButtonInner} aria-hidden="true"><Plus size={20} /></span>
          <span className={styles.navItemLabel}>New</span>
        </button>
        <button
          className={`${styles.navItem} ${isMoreActive ? styles.navItemActive : ""}`}
          onClick={() => setMoreOpen((o) => !o)}
          aria-label="More navigation options"
          aria-expanded={moreOpen}
        >
          <span className={styles.navItemIcon} aria-hidden="true"><MoreHorizontal size={20} /></span>
          <span className={styles.navItemLabel}>More</span>
        </button>
      </nav>
    </>
  );
}
