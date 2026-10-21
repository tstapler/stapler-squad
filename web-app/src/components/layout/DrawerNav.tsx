"use client";

import React from "react";
import { Link, usePathname } from "@/lib/nav";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useNavigation } from "@/lib/contexts/NavigationContext";
import { NAV_PAGES, groupNavPages, NAV_GROUP_LABELS } from "@/lib/nav-pages";
import type { NavGroup } from "@/lib/nav-pages";
import { routes } from "@/lib/routes";
import { useFeatureFlags } from "@/lib/contexts/FeatureFlagsContext";
import { ReviewQueueNavBadge } from "@/components/sessions/ReviewQueueNavBadge";
import { UnfinishedNavBadge } from "@/components/unfinished/UnfinishedNavBadge";
import { StuckNavBadge } from "@/components/backlog-stuck/StuckNavBadge";
import { NotificationsNavBadge } from "@/components/ui/NotificationsNavBadge";
import { Tooltip } from "@/components/ui/Tooltip";
import {
  drawer,
  navList,
  navItem,
  navIcon,
  navLabel,
  navBadgeWrapper,
  toggleButton,
  drawerDivider,
  sectionHeader,
  sectionSpacer,
} from "./DrawerNav.css";

export function DrawerNav() {
  const { isDrawerOpen, toggleDrawer } = useNavigation();
  const pathname = usePathname();
  const { flags } = useFeatureFlags();

  const visiblePages = NAV_PAGES.filter((p) => !p.featureFlag || flags[p.featureFlag]);
  const groups = groupNavPages(visiblePages);

  return (
    <nav
      className={drawer({ open: isDrawerOpen })}
      data-testid="drawer-nav"
      aria-label="Main navigation"
    >
      <ul className={navList} role="list">
        {Array.from(groups.entries()).map(([group, pages], groupIndex) => (
          <React.Fragment key={group}>
            {groupIndex > 0 && (
              <li role="presentation" aria-hidden="true">
                <div className={sectionSpacer} />
              </li>
            )}
            <li role="presentation" aria-hidden="true">
              <span className={sectionHeader({ visible: isDrawerOpen })}>
                {NAV_GROUP_LABELS[group]}
              </span>
            </li>
            {pages.map((page) => {
              const isActive =
                page.href === routes.home
                  ? pathname === routes.home
                  : pathname.startsWith(page.href);

              const Icon = page.icon;
              return (
                <li key={page.href}>
                  <NavTooltip label={page.label} enabled={!isDrawerOpen}>
                  <Link
                    href={page.href}
                    className={navItem({ active: isActive })}
                    aria-current={isActive ? "page" : undefined}
                  >
                    <span className={navIcon} aria-hidden="true">
                      <Icon size={18} />
                    </span>
                    <span className={navLabel({ visible: isDrawerOpen })}>
                      {page.label}
                    </span>
                    {page.href === routes.reviewQueue && (
                      <span className={navBadgeWrapper({ collapsed: !isDrawerOpen })}>
                        <ReviewQueueNavBadge inline={isDrawerOpen} />
                      </span>
                    )}
                    {page.href === routes.unfinished && (
                      <span className={navBadgeWrapper({ collapsed: !isDrawerOpen })}>
                        <UnfinishedNavBadge inline={isDrawerOpen} />
                        <StuckNavBadge inline={isDrawerOpen} />
                      </span>
                    )}
                    {page.href === routes.notifications && (
                      <span className={navBadgeWrapper({ collapsed: !isDrawerOpen })}>
                        <NotificationsNavBadge inline={isDrawerOpen} />
                      </span>
                    )}
                  </Link>
                  </NavTooltip>
                </li>
              );
            })}
          </React.Fragment>
        ))}
      </ul>

      <div className={drawerDivider} />

      <button
        className={toggleButton}
        onClick={toggleDrawer}
        aria-label="Toggle navigation"
        aria-expanded={isDrawerOpen}
        title={isDrawerOpen ? "Collapse navigation" : "Expand navigation"}
      >
        <span aria-hidden="true">
          {isDrawerOpen ? <ChevronLeft size={16} /> : <ChevronRight size={16} />}
        </span>
      </button>
    </nav>
  );
}

// Tooltip only while the drawer is collapsed; expanded items already show their label.
function NavTooltip({ label, enabled, children }: { label: string; enabled: boolean; children: React.ReactElement }) {
  return enabled ? (
    <Tooltip label={label} side="right">
      {children}
    </Tooltip>
  ) : (
    children
  );
}
