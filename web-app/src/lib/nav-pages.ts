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
}

export const NAV_PAGES: NavPage[] = [
  { href: routes.home,        label: "Sessions",     icon: "⊞" },
  { href: routes.unfinished,  label: "Unfinished",   icon: "✎" },
  { href: routes.reviewQueue, label: "Review Queue", shortLabel: "Review", icon: "📋" },
  { href: routes.rules,       label: "Rules",        icon: "📜" },
  { href: routes.logs,        label: "Logs",         icon: "📋", mobileNav: false },
  { href: routes.errors,      label: "Errors",       icon: "⚠", mobileNav: false },
  { href: routes.history,     label: "History",      icon: "🕐" },
  { href: routes.config,      label: "Config",       icon: "⚙" },
  { href: routes.settings,    label: "Settings",     icon: "⚙", mobileNav: false },
];

export const MOBILE_NAV_PAGES = NAV_PAGES.filter((p) => p.mobileNav !== false);
