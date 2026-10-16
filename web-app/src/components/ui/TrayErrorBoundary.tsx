"use client";

import Link from "next/link";
import type { ReactNode } from "react";
import { routes } from "@/lib/routes";
import { ErrorBoundary } from "./ErrorBoundary";
import { TrayHandle } from "./TrayHandle";
import { trayErrorLink } from "./NotificationPanel.css";

function TrayUnavailable() {
  return (
    <>
      <TrayHandle />
      <Link href={routes.notifications} className={trayErrorLink} data-testid="tray-error-link">
        Notifications unavailable - Open Notifications page
      </Link>
    </>
  );
}

/**
 * Keeps a render error in the tray from reaching the app-wide boundary: the page
 * stays up, the handle stays usable and a 44px link leads to the Notifications page.
 */
export function TrayErrorBoundary({ children }: { children: ReactNode }) {
  return <ErrorBoundary fallback={<TrayUnavailable />}>{children}</ErrorBoundary>;
}
