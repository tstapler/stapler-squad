"use client";

import { useState } from "react";
import { AppLink } from "@/components/ui/AppLink";
import { usePathname } from "next/navigation";
import { ReviewQueueNavBadge } from "@/components/sessions/ReviewQueueNavBadge";
import { ApprovalNavBadge } from "@/components/sessions/ApprovalNavBadge";
import { DebugMenu } from "@/components/ui/DebugMenu";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { useOmnibar } from "@/lib/contexts/OmnibarContext";
import styles from "./Header.module.css";

export function Header() {
  const pathname = usePathname();
  const [isDebugMenuOpen, setIsDebugMenuOpen] = useState(false);
  const { togglePanel, getUnreadCount } = useNotifications();
  const { open: openOmnibar } = useOmnibar();
  const unreadCount = getUnreadCount();

  return (
    <>
      <header className={styles.header}>
      <div className={styles.container}>
        <div className={styles.branding}>
          <h1 className={styles.title}>Claude Squad</h1>
          <span className={styles.subtitle}>Session Manager</span>
        </div>

        <nav className={styles.nav}>
          <AppLink
            href="/"
            className={`${styles.navLink} ${pathname === "/" ? styles.active : ""}`}
            onClick={() => console.log("Sessions link clicked")}
          >
            Sessions
          </AppLink>
          <AppLink
            href="/review-queue"
            className={`${styles.navLink} ${pathname === "/review-queue" ? styles.active : ""}`}
            onClick={() => console.log("Review Queue link clicked")}
          >
            <span className={styles.navLinkText}>Review Queue</span>
            <ReviewQueueNavBadge inline={true} />
          </AppLink>
          <AppLink
            href="/logs"
            className={`${styles.navLink} ${pathname === "/logs" ? styles.active : ""}`}
            onClick={() => console.log("Logs link clicked")}
          >
            Logs
          </AppLink>
          <AppLink
            href="/history"
            className={`${styles.navLink} ${pathname === "/history" ? styles.active : ""}`}
            onClick={() => console.log("History link clicked")}
          >
            History
          </AppLink>
          <AppLink
            href="/config"
            className={`${styles.navLink} ${pathname === "/config" ? styles.active : ""}`}
            onClick={() => console.log("Config link clicked")}
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
            <span className={styles.newSessionIcon}>+</span>
            New Session
          </button>
          <ApprovalNavBadge />
          <button
            className={styles.notificationButton}
            onClick={togglePanel}
            aria-label="Open notifications"
            title="Notifications"
          >
            🔔
            {unreadCount > 0 && (
              <span className={styles.notificationBadge}>{unreadCount}</span>
            )}
          </button>
          <button
            className={styles.debugButton}
            onClick={() => setIsDebugMenuOpen(true)}
            aria-label="Open debug menu"
            title="Debug menu"
          >
            🛠️
          </button>
          <button
            className={styles.helpButton}
            onClick={() => {
              // Will be wired up to show keyboard shortcuts
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
        isOpen={isDebugMenuOpen}
        onClose={() => setIsDebugMenuOpen(false)}
      />
    </>
  );
}
