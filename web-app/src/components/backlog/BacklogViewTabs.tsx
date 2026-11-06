"use client";

import { AppLink } from "@/components/ui/AppLink";
import * as styles from "./BacklogViewTabs.css";

export type BacklogView = "list" | "board" | "flow";

const TABS: readonly { view: BacklogView; label: string; href: string }[] = [
  { view: "list", label: "List", href: "/backlog" },
  { view: "board", label: "Board", href: "/backlog/board" },
  { view: "flow", label: "Flow", href: "/backlog/flow" },
];

/** The List / Board / Flow strip shared by every backlog view page. */
export function BacklogViewTabs({ active }: { active: BacklogView }) {
  return (
    <nav className={`${styles.tabBar}`} aria-label="Backlog views">
      {TABS.map((tab) => {
        const isActive = tab.view === active;
        return (
          <AppLink
            key={tab.view}
            href={tab.href}
            className={isActive ? `${styles.tab} ${styles.tabActive}` : `${styles.tab}`}
            aria-current={isActive ? "page" : undefined}
            data-testid={`backlog-tab-${tab.view}`}
          >
            {tab.label}
          </AppLink>
        );
      })}
    </nav>
  );
}
