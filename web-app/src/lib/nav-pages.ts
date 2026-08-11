import type { LucideIcon } from "lucide-react";
import {
  LayoutGrid,
  Clock4,
  ClipboardCheck,
  Bell,
  Settings,
  BookOpen,
  History,
  ScrollText,
  AlertTriangle,
  HelpCircle,
  BarChart2,
  LayoutList,
  Zap,
  FolderOpen,
  ToggleLeft,
} from "lucide-react";
import { routes } from "./routes";

export interface NavPage {
  href: string;
  /** Full label used in Header desktop nav */
  label: string;
  /** Abbreviated label for BottomNav primary bar (falls back to label) */
  shortLabel?: string;
  /** Lucide icon component */
  icon: LucideIcon;
  /** Set false to exclude from BottomNav entirely (desktop-only routes) */
  mobileNav?: boolean;
  /** Set false to hide from the always-visible header nav row (still in hamburger) */
  headerNav?: boolean;
  /** True = rendered in the BottomNav primary bar; absent = goes into the More sheet */
  bottomNavPrimary?: boolean;
  /** Feature flag name that must be enabled for this page to appear in nav */
  featureFlag?: string;
  /** Navigation group for grouped rendering in DrawerNav and BottomNav More sheet */
  group: NavGroup;
}

export type NavGroup = "work" | "automation" | "insights" | "settings";
export const NAV_GROUP_LABELS: Record<NavGroup, string> = {
  work: "Work",
  automation: "Automation",
  insights: "Insights",
  settings: "Settings & Tools",
};
/** Render order for grouped nav surfaces. Lower index = rendered first. */
export const NAV_GROUP_SORT_ORDER: NavGroup[] = ["settings", "work", "automation", "insights"];

export const NAV_PAGES: NavPage[] = [
  { href: routes.home,          label: "Sessions",      icon: LayoutGrid,     bottomNavPrimary: true, group: "work" },
  { href: routes.backlog,       label: "Backlog",       icon: LayoutList,     bottomNavPrimary: true, featureFlag: "backlog", group: "work" },
  { href: routes.unfinished,    label: "Up Next",       icon: Clock4,         bottomNavPrimary: true, group: "work" },
  { href: routes.reviewQueue,   label: "Review Queue",  shortLabel: "Review", icon: ClipboardCheck, bottomNavPrimary: true, group: "work" },
  // Notifications is custom-rendered in BottomNav (badge logic) — marked primary to keep it out of the More sheet
  { href: routes.notifications, label: "Notifications", shortLabel: "Alerts", icon: Bell, bottomNavPrimary: true, group: "work" },
  { href: routes.settings,      label: "Settings",      icon: Settings, headerNav: false, group: "settings" },
  // Secondary — hamburger / More-sheet only
  { href: routes.insights, label: "Insights", icon: BarChart2, headerNav: false, group: "insights" },
  { href: routes.workflows, label: "Workflows", icon: Zap,             headerNav: false, group: "automation" },
  { href: routes.rules,   label: "Rules",   icon: BookOpen,          headerNav: false, group: "automation" },
  { href: routes.history, label: "History", icon: History,           headerNav: false, group: "insights" },
  { href: routes.logs,    label: "Logs",    icon: ScrollText,  headerNav: false, group: "settings" },
  { href: routes.errors,  label: "Errors",  icon: AlertTriangle, headerNav: false, group: "settings" },
  { href: routes.help,    label: "Help",    icon: HelpCircle,  headerNav: false, group: "settings" },
  { href: routes.escapeAnalytics, label: "Escape Analytics", icon: BarChart2, headerNav: false, group: "insights" },
  { href: routes.files,           label: "Files",            icon: FolderOpen,   headerNav: false, group: "settings" },
  { href: routes.settingsFeatures, label: "Feature Flags",   icon: ToggleLeft,   headerNav: false, group: "settings" },
];

export const MOBILE_NAV_PAGES = NAV_PAGES.filter((p) => p.mobileNav !== false);
/** Items shown in the always-visible header nav row on wide desktop (≥1100px). */
export const HEADER_NAV_PAGES = NAV_PAGES.filter((p) => p.headerNav !== false);
/** Items rendered in the BottomNav primary bar (excluding Notifications which is custom-rendered). */
export const BOTTOM_NAV_PRIMARY = NAV_PAGES.filter(
  (p) => p.bottomNavPrimary && p.mobileNav !== false && p.href !== routes.notifications
);
/** Items that fall into the BottomNav More sheet (mobile-visible, not primary). */
export const BOTTOM_NAV_MORE = NAV_PAGES.filter(
  (p) => p.mobileNav !== false && !p.bottomNavPrimary
);

export function groupNavPages(pages: NavPage[]): Map<NavGroup, NavPage[]> {
  const map = new Map<NavGroup, NavPage[]>();
  for (const page of pages) {
    const existing = map.get(page.group);
    if (existing) {
      existing.push(page);
    } else {
      map.set(page.group, [page]);
    }
  }
  return map;
}
