"use client";
// +feature: ui:bottom-nav

import { useState, useEffect, useRef } from "react";
import { AppLink } from "@/components/ui/AppLink";
import { usePathname } from "next/navigation";
import { ReviewQueueNavBadge } from "@/components/sessions/ReviewQueueNavBadge";
import { useOmnibar } from "@/lib/contexts/OmnibarContext";
import { useAuth } from "@/lib/contexts/AuthContext";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { routes } from "@/lib/routes";
import * as styles from "./BottomNav.css";

const primaryItems = [
  { href: routes.home, label: "Sessions", icon: "⊞" },
  { href: routes.unfinished, label: "Unfinished", icon: "✎" },
  { href: routes.reviewQueue, label: "Review", icon: "📋" },
] as const;

const moreItems = [
  { href: routes.history, label: "History", icon: "🕐" },
  { href: routes.logs, label: "Logs", icon: "📄" },
  { href: routes.rules, label: "Rules", icon: "📜" },
  { href: routes.config, label: "Config", icon: "⚙" },
  { href: routes.settings, label: "Settings", icon: "🔧" },
] as const;

type PrimaryItem = (typeof primaryItems)[number];
type MoreItem = (typeof moreItems)[number];

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
              {item.icon}
              <ReviewQueueNavBadge inline={true} />
            </>
          ) : (
            item.icon
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
          return (
            <AppLink
              key={item.href}
              href={item.href}
              className={`${styles.moreSheetItem} ${isActive ? styles.moreSheetItemActive : ""}`}
              aria-current={isActive ? "page" : undefined}
            >
              <span className={styles.moreSheetItemIcon} aria-hidden="true">{item.icon}</span>
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
            <span className={styles.moreSheetItemIcon} aria-hidden="true">👤</span>
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
            <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/>
              <path d="M13.73 21a2 2 0 0 1-3.46 0"/>
            </svg>
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
          <span className={styles.newSessionButtonInner} aria-hidden="true">+</span>
          <span className={styles.navItemLabel}>New</span>
        </button>
        <button
          className={`${styles.navItem} ${isMoreActive ? styles.navItemActive : ""}`}
          onClick={() => setMoreOpen((o) => !o)}
          aria-label="More navigation options"
          aria-expanded={moreOpen}
        >
          <span className={styles.navItemIcon} aria-hidden="true">⋯</span>
          <span className={styles.navItemLabel}>More</span>
        </button>
      </nav>
    </>
  );
}
