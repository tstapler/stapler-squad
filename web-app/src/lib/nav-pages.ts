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
} from "lucide-react";
import { routes } from "./routes";

export interface NavPage {
  href: string;
  /** Full label used in Header desktop nav */
  label: string;
  /** Abbreviated label for BottomNav (falls back to label) */
  shortLabel?: string;
  /** Lucide icon component */
  icon: LucideIcon;
  /** Set false to exclude from BottomNav (desktop-only routes) */
  mobileNav?: boolean;
  /** Set false to hide from the always-visible header nav row (still in hamburger) */
  headerNav?: boolean;
}

export const NAV_PAGES: NavPage[] = [
  { href: routes.home,          label: "Sessions",      icon: LayoutGrid },
  { href: routes.unfinished,    label: "Unfinished",    icon: Clock4 },
  { href: routes.reviewQueue,   label: "Review Queue",  shortLabel: "Review", icon: ClipboardCheck },
  { href: routes.notifications, label: "Notifications", shortLabel: "Alerts", icon: Bell },
  { href: routes.settings,      label: "Settings",      icon: Settings, mobileNav: false },
  // Secondary — hamburger / More-sheet only
  { href: routes.rules,   label: "Rules",   icon: BookOpen,          headerNav: false },
  { href: routes.history, label: "History", icon: History,           headerNav: false },
  { href: routes.settings + "?tab=config-files", label: "Config Files", icon: SlidersHorizontal, headerNav: false },
  { href: routes.logs,    label: "Logs",    icon: ScrollText,  mobileNav: false, headerNav: false },
  { href: routes.errors,  label: "Errors",  icon: AlertTriangle, mobileNav: false, headerNav: false },
  { href: routes.help,    label: "Help",    icon: HelpCircle,  mobileNav: false, headerNav: false },
  { href: routes.escapeAnalytics, label: "Escape Analytics", icon: BarChart2, mobileNav: false, headerNav: false },
];

export const MOBILE_NAV_PAGES = NAV_PAGES.filter((p) => p.mobileNav !== false);
/** Items shown in the always-visible header nav row on wide desktop (≥1100px). */
export const HEADER_NAV_PAGES = NAV_PAGES.filter((p) => p.headerNav !== false);
