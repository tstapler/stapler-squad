import type { LucideIcon } from "lucide-react";
import {
  LayoutGrid,
  Clock4,
  ClipboardCheck,
  Bell,
  Settings,
  BookOpen,
  History,
  SlidersHorizontal,
  ScrollText,
  AlertTriangle,
  HelpCircle,
  BarChart2,
  LayoutList,
  Zap,
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
}

export const NAV_PAGES: NavPage[] = [
  { href: routes.home,          label: "Sessions",      icon: LayoutGrid,     bottomNavPrimary: true },
  { href: routes.backlog,       label: "Backlog",       icon: LayoutList,     bottomNavPrimary: true, featureFlag: "backlog" },
  { href: routes.unfinished,    label: "Unfinished",    icon: Clock4,         bottomNavPrimary: true },
  { href: routes.reviewQueue,   label: "Review Queue",  shortLabel: "Review", icon: ClipboardCheck, bottomNavPrimary: true },
  // Notifications is custom-rendered in BottomNav (badge logic) — marked primary to keep it out of the More sheet
  { href: routes.notifications, label: "Notifications", shortLabel: "Alerts", icon: Bell, bottomNavPrimary: true },
  { href: routes.settings,      label: "Settings",      icon: Settings, mobileNav: false },
  // Secondary — hamburger / More-sheet only
  { href: routes.insights, label: "Insights", icon: BarChart2, mobileNav: false, headerNav: false },
  { href: routes.workflows, label: "Workflows", icon: Zap,             headerNav: false },
  { href: routes.rules,   label: "Rules",   icon: BookOpen,          headerNav: false },
  { href: routes.history, label: "History", icon: History,           headerNav: false },
  { href: routes.settings + "?tab=config-files", label: "Config Files", icon: SlidersHorizontal, headerNav: false },
  { href: routes.settingsFeatures, label: "Features", icon: Settings, headerNav: false },
  { href: routes.logs,    label: "Logs",    icon: ScrollText,  mobileNav: false, headerNav: false },
  { href: routes.errors,  label: "Errors",  icon: AlertTriangle, mobileNav: false, headerNav: false },
  { href: routes.help,    label: "Help",    icon: HelpCircle,  mobileNav: false, headerNav: false },
  { href: routes.escapeAnalytics, label: "Escape Analytics", icon: BarChart2, mobileNav: false, headerNav: false },
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
