"use client";
// +feature: ui:bottom-nav

import { useState, useEffect, useRef } from "react";
import {
  Bell,
  Plus,
  MoreHorizontal,
  User,
  Hand,
} from "lucide-react";
import { AppLink } from "@/components/ui/AppLink";
import { usePathname } from "next/navigation";
import { ReviewQueueNavBadge } from "@/components/sessions/ReviewQueueNavBadge";
import { useOmnibar } from "@/lib/contexts/OmnibarContext";
import { useAuth } from "@/lib/contexts/AuthContext";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { routes } from "@/lib/routes";
import { BOTTOM_NAV_PRIMARY, BOTTOM_NAV_MORE, type NavPage } from "@/lib/nav-pages";
import * as styles from "./BottomNav.css";

const HANDEDNESS_KEY = "stapler-squad:left-handed";

export function BottomNav() {
  const pathname = usePathname();
  const { open: openOmnibar } = useOmnibar();
  const { authenticated, authEnabled } = useAuth();
  const { getUnreadCount } = useNotifications();
  const unreadCount = getUnreadCount();
  const [moreOpen, setMoreOpen] = useState(false);
  const [leftHanded, setLeftHanded] = useState(false);

  // Load handedness preference from localStorage (client-side only)
  useEffect(() => {
    try {
      setLeftHanded(localStorage.getItem(HANDEDNESS_KEY) === "true");
    } catch { /* ignore */ }
  }, []);

  const toggleHandedness = () => {
    const next = !leftHanded;
    setLeftHanded(next);
    try { localStorage.setItem(HANDEDNESS_KEY, String(next)); } catch { /* ignore */ }
  };

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

  const isMoreActive = BOTTOM_NAV_MORE.some((item) => pathname?.startsWith(item.href));

  const renderPrimaryItem = (item: NavPage) => {
    const isActive =
      item.href === routes.home
        ? pathname === routes.home
        : pathname?.startsWith(item.href);
    const Icon = item.icon;
    const label = item.shortLabel ?? item.label;

    return (
      <AppLink
        key={item.href}
        href={item.href}
        className={`${styles.navItem} ${isActive ? styles.navItemActive : ""}`}
        aria-current={isActive ? "page" : undefined}
      >
        <span className={styles.navItemIcon} aria-hidden="true">
          {item.href === routes.reviewQueue ? (
            <>
              <Icon size={20} />
              <ReviewQueueNavBadge inline={true} />
            </>
          ) : (
            <Icon size={20} />
          )}
        </span>
        <span className={styles.navItemLabel}>{label}</span>
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
        {BOTTOM_NAV_MORE.map((item) => {
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
        <button
          className={styles.moreSheetItem}
          onClick={toggleHandedness}
          style={{ width: "100%", textAlign: "left", cursor: "pointer", border: "none", background: "transparent" }}
          aria-pressed={leftHanded}
        >
          <span className={styles.moreSheetItemIcon} aria-hidden="true"><Hand size={20} /></span>
          <span>{leftHanded ? "Switch to right-handed" : "Switch to left-handed"}</span>
        </button>
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
      <nav
        ref={navRef}
        className={styles.nav}
        aria-label="Bottom navigation"
        data-left-handed={leftHanded || undefined}
      >
        {BOTTOM_NAV_PRIMARY.map(renderPrimaryItem)}
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
