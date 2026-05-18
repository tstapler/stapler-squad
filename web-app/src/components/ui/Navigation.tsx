"use client";

import { AppLink } from "@/components/ui/AppLink";
import { usePathname } from "next/navigation";
import { routes } from "@/lib/routes";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import {
  nav,
  container,
  brand,
  navTitle,
  menu,
  link,
  active,
  actions,
  createButton,
} from "./Navigation.css";

export function Navigation() {
  const pathname = usePathname();
  const backlogEnabled = useFeatureFlag("backlog");

  const navItems = [
    { href: routes.home, label: "Sessions" },
    { href: routes.reviewQueue, label: "Review Queue" },
    ...(backlogEnabled ? [{ href: routes.backlog, label: "Backlog" }] : []),
  ];

  return (
    <nav className={nav} role="navigation" aria-label="Main navigation">
      <div className={container}>
        <div className={brand}>
          <AppLink href={routes.home} aria-label="Stapler Squad home">
            <h1 className={navTitle}>Stapler Squad</h1>
          </AppLink>
        </div>

        <ul className={menu} role="menubar">
          {navItems.map((item) => (
            <li key={item.href} role="none">
              <AppLink
                href={item.href}
                role="menuitem"
                aria-current={pathname === item.href ? "page" : undefined}
                className={`${link} ${
                  pathname === item.href ? active : ""
                }`}
              >
                {item.label}
              </AppLink>
            </li>
          ))}
        </ul>

        <div className={actions}>
          <AppLink
            href={routes.sessionCreate}
            className={createButton}
            aria-label="Create new session"
          >
            New Session
          </AppLink>
        </div>
      </div>
    </nav>
  );
}
