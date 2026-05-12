import { routes } from "./routes";

export interface NavPage {
  href: string;
  /** Full label used in Header desktop nav */
  label: string;
  /** Abbreviated label for BottomNav (falls back to label) */
  shortLabel?: string;
  /** Icon for BottomNav */
  icon: string;
  /** Set false to exclude from BottomNav (desktop-only routes) */
  mobileNav?: boolean;
  /** Set false to hide from the always-visible header nav row (still in hamburger) */
  headerNav?: boolean;
}

export const NAV_PAGES: NavPage[] = [
  { href: routes.home,          label: "Sessions",      icon: "⊞" },
  { href: routes.backlog,       label: "Backlog",       icon: "☰" },
  { href: routes.unfinished,    label: "Unfinished",    icon: "✎" },
  { href: routes.reviewQueue,   label: "Review Queue",  shortLabel: "Review", icon: "📋" },
  { href: routes.notifications, label: "Notifications", shortLabel: "Alerts", icon: "🔔" },
  { href: routes.settings,      label: "Settings",      icon: "⚙", mobileNav: false },
  // Secondary — hamburger / More-sheet only
  { href: routes.rules,   label: "Rules",   icon: "📜", headerNav: false },
  { href: routes.history, label: "History", icon: "🕐", headerNav: false },
  { href: routes.config,  label: "Config",  icon: "⚙", headerNav: false },
  { href: routes.logs,    label: "Logs",    icon: "📋", mobileNav: false, headerNav: false },
  { href: routes.errors,  label: "Errors",  icon: "⚠",  mobileNav: false, headerNav: false },
];

export const MOBILE_NAV_PAGES = NAV_PAGES.filter((p) => p.mobileNav !== false);
/** Items shown in the always-visible header nav row on wide desktop (≥1100px). */
export const HEADER_NAV_PAGES = NAV_PAGES.filter((p) => p.headerNav !== false);
