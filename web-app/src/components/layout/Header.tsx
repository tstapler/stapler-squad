"use client";

import { useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { ReviewQueueNavBadge } from "@/components/sessions/ReviewQueueNavBadge";
import { DebugMenu } from "@/components/ui/DebugMenu";
import styles from "./Header.module.css";

export function Header() {
  const pathname = usePathname();
  const [isDebugMenuOpen, setIsDebugMenuOpen] = useState(false);

  return (
    <>
      <header className={styles.header}>
      <div className={styles.container}>
        <div className={styles.branding}>
          <h1 className={styles.title}>Claude Squad</h1>
          <span className={styles.subtitle}>Session Manager</span>
        </div>

        <nav className={styles.nav}>
          <Link
            href="/"
            className={`${styles.navLink} ${pathname === "/" ? styles.active : ""}`}
            onClick={() => console.log("Sessions link clicked")}
          >
            Sessions
          </Link>
          <Link
            href="/review-queue"
            className={`${styles.navLink} ${pathname === "/review-queue" ? styles.active : ""}`}
            onClick={() => console.log("Review Queue link clicked")}
          >
            <span className={styles.navLinkText}>Review Queue</span>
            <ReviewQueueNavBadge inline={true} />
          </Link>
        </nav>

        <div className={styles.actions}>
          <Link href="/sessions/new" className={styles.newSessionButton}>
            <span className={styles.newSessionIcon}>+</span>
            New Session
          </Link>
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
