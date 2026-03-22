"use client";

import { useState, useEffect } from "react";
import { AppLink } from "@/components/ui/AppLink";
import { usePathname } from "next/navigation";
import { ReviewQueueNavBadge } from "@/components/sessions/ReviewQueueNavBadge";
import { ApprovalNavBadge } from "@/components/sessions/ApprovalNavBadge";
import { DebugMenu } from "@/components/ui/DebugMenu";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { useOmnibar } from "@/lib/contexts/OmnibarContext";
import { routes } from "@/lib/routes";
import styles from "./Header.module.css";

export function Header() {
  const pathname = usePathname();
  const [isDebugOpen, setIsDebugOpen] = useState(false);
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
  const { togglePanel, getUnreadCount } = useNotifications();
  const { open: openOmnibar } = useOmnibar();
  const unreadCount = getUnreadCount();

  // Close mobile menu on Escape
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isMobileMenuOpen) {
        setIsMobileMenuOpen(false);
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [isMobileMenuOpen]);

  // Close mobile menu on route change
  useEffect(() => {
    setIsMobileMenuOpen(false);
  }, [pathname]);

  return (
    <>
      <header className={styles.header}>
      <div className={styles.container}>
        <div className={styles.branding}>
          <h1 className={styles.title}>Stapler Squad</h1>
          <span className={styles.subtitle}>Session Manager</span>
        </div>

        <button
          className={styles.hamburger}
          aria-label={isMobileMenuOpen ? "Close navigation menu" : "Open navigation menu"}
          aria-expanded={isMobileMenuOpen}
          aria-controls="mobile-nav"
          onClick={() => setIsMobileMenuOpen((prev) => !prev)}
        >
          <span className={`${styles.hamburgerLine} ${isMobileMenuOpen ? styles.hamburgerLineOpen1 : ""}`} />
          <span className={`${styles.hamburgerLine} ${isMobileMenuOpen ? styles.hamburgerLineOpen2 : ""}`} />
          <span className={`${styles.hamburgerLine} ${isMobileMenuOpen ? styles.hamburgerLineOpen3 : ""}`} />
        </button>

        <nav
          id="mobile-nav"
          aria-label="Main navigation"
          className={`${styles.nav} ${isMobileMenuOpen ? styles.navOpen : ""}`}
        >
          <AppLink
            href={routes.home}
            className={`${styles.navLink} ${pathname === routes.home ? styles.active : ""}`}
          >
            Sessions
          </AppLink>
          <AppLink
            href={routes.reviewQueue}
            className={`${styles.navLink} ${pathname === routes.reviewQueue ? styles.active : ""}`}
          >
            <span className={styles.navLinkText}>Review Queue</span>
            <ReviewQueueNavBadge inline={true} />
          </AppLink>
          <AppLink
            href={routes.logs}
            className={`${styles.navLink} ${pathname === routes.logs ? styles.active : ""}`}
          >
            Logs
          </AppLink>
          <AppLink
            href={routes.history}
            className={`${styles.navLink} ${pathname === routes.history ? styles.active : ""}`}
          >
            History
          </AppLink>
          <AppLink
            href={routes.config}
            className={`${styles.navLink} ${pathname === routes.config ? styles.active : ""}`}
          >
            Config
          </AppLink>
        </nav>

        <div className={styles.actions}>
          <button
            className={styles.newSessionButton}
            onClick={openOmnibar}
            aria-label="Create new session (⌘K)"
            title="Create new session (⌘K)"
          >
            <span className={styles.newSessionIcon} aria-hidden="true">+</span>
            <span className={styles.newSessionLabel}>New Session</span>
          </button>
          <ApprovalNavBadge />
          <button
            className={styles.notificationButton}
            onClick={togglePanel}
            aria-label="Open notifications"
            title="Notifications"
          >
            <svg aria-hidden="true" xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/>
              <path d="M13.73 21a2 2 0 0 1-3.46 0"/>
            </svg>
            {unreadCount > 0 && (
              <span className={styles.notificationBadge} aria-label={`${unreadCount} unread`}>{unreadCount}</span>
            )}
          </button>
          <button
            className={styles.debugButton}
            onClick={() => setIsDebugOpen(true)}
            aria-label="Open debug menu"
            title="Debug menu"
          >
            <svg aria-hidden="true" xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/>
            </svg>
          </button>
          <button
            className={styles.helpButton}
            onClick={() => {
              window.dispatchEvent(new KeyboardEvent("keydown", { key: "?" }));
            }}
            aria-label="Show keyboard shortcuts"
            title="Keyboard shortcuts (?)"
          >
            ?
          </button>
        </div>
      </div>
    </header>

      <DebugMenu
        isOpen={isDebugOpen}
        onClose={() => setIsDebugOpen(false)}
      />
    </>
  );
}
