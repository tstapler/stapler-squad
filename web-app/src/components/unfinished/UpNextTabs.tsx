"use client";

// +feature: up-next-tabs

import type { ReactNode } from "react";
import * as Tabs from "@radix-ui/react-tabs";
import { UP_NEXT_TABS, type UpNextTab } from "@/lib/unfinished/upNextTab";
import * as styles from "./UpNextTabs.css";

const TAB_LABELS: Record<UpNextTab, string> = {
  prs: "PRs",
  stuck: "Stuck",
  worktrees: "Worktrees",
  queue: "Queue",
};

const PANEL_HEADINGS: Record<UpNextTab, string> = {
  prs: "Open pull requests",
  stuck: "Stuck backlog items",
  worktrees: "Worktrees",
  queue: "Backlog queue",
};

export interface UpNextTabsProps {
  value: UpNextTab;
  onValueChange: (tab: UpNextTab) => void;
  /** Count of items needing attention; 0 or undefined hides the badge. */
  badges?: Partial<Record<UpNextTab, number>>;
  /** Badge count is a lower bound (rendered "N+"). */
  degraded?: Partial<Record<UpNextTab, boolean>>;
  /** Explanatory text exposed as tooltip and aria-describedby on the tab. */
  descriptions?: Partial<Record<UpNextTab, string>>;
  /** Panel body for each tab; only the active one is mounted. */
  panels: Partial<Record<UpNextTab, ReactNode>>;
}

function tabName(tab: UpNextTab, count: number, isDegraded: boolean): string {
  const label = TAB_LABELS[tab];
  if (count <= 0) return label;
  return `${label}, ${count}${isDegraded ? " or more" : ""} need attention`;
}

export function UpNextTabs({
  value,
  onValueChange,
  badges = {},
  degraded = {},
  descriptions = {},
  panels,
}: UpNextTabsProps) {
  return (
    <Tabs.Root
      value={value}
      onValueChange={(next) => onValueChange(next as UpNextTab)}
      activationMode="manual"
    >
      <Tabs.List className={styles.tabList} aria-label="Up next sections">
        {UP_NEXT_TABS.map((tab) => {
          const count = badges[tab] ?? 0;
          const isDegraded = degraded[tab] === true;
          const description = descriptions[tab];
          const descriptionId = description ? `up-next-tab-desc-${tab}` : undefined;
          return (
            <Tabs.Trigger
              key={tab}
              value={tab}
              className={styles.tab}
              data-testid={`up-next-tab-${tab}`}
              aria-label={tabName(tab, count, isDegraded)}
              aria-describedby={descriptionId}
              title={description}
            >
              {TAB_LABELS[tab]}
              {count > 0 && (
                <span className={styles.badge} aria-hidden="true">
                  {count}
                  {isDegraded ? "+" : ""}
                </span>
              )}
              {descriptionId && (
                <span id={descriptionId} className={styles.visuallyHidden}>
                  {description}
                </span>
              )}
            </Tabs.Trigger>
          );
        })}
      </Tabs.List>
      {UP_NEXT_TABS.map((tab) => (
        <Tabs.Content
          key={tab}
          value={tab}
          className={styles.panel}
          tabIndex={0}
          data-testid={`up-next-panel-${tab}`}
        >
          <h2 className={styles.panelHeading} tabIndex={-1}>
            {PANEL_HEADINGS[tab]}
          </h2>
          {panels[tab]}
        </Tabs.Content>
      ))}
    </Tabs.Root>
  );
}
