import type { ReactNode, RefObject } from "react";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import { AuthProvider } from "@/lib/contexts/AuthContext";
import { Providers } from "./Providers";
import { NotificationPanel } from "@/components/ui/NotificationPanel";
import { TrayErrorBoundary } from "@/components/ui/TrayErrorBoundary";
import { TmuxVersionMismatchBanner } from "@/components/system/TmuxVersionMismatchBanner";
import { ForkPressureStatusBanner } from "@/components/system/ForkPressureStatusBanner";
import { ViewportProvider } from "@/components/providers/ViewportProvider";
import { CockpitShell } from "@/components/layout/CockpitShell";
import { WindowsProvider } from "@/lib/window/WindowsProvider";
import { mainContent } from "./layout.css";

/**
 * Provider and chrome tree shared by every entry point (the Next layout now, the Bun entry later).
 * `mainRef` is the router build's focus target after navigation (see `useFocusOnNavigate`).
 */
export function RootShell({ children, mainRef }: { children: ReactNode; mainRef?: RefObject<HTMLElement | null> }) {
  return (
    <ViewportProvider>
      <ErrorBoundary>
        <AuthProvider>
          <Providers>
            <CockpitShell>
              <WindowsProvider>
                <a href="#main-content" className="skip-link">Skip to main content</a>
                <main id="main-content" ref={mainRef} tabIndex={-1} className={mainContent}>
                  <TmuxVersionMismatchBanner />
                  <ForkPressureStatusBanner />
                  {children}
                </main>
                <TrayErrorBoundary>
                  <NotificationPanel />
                </TrayErrorBoundary>
              </WindowsProvider>
            </CockpitShell>
          </Providers>
        </AuthProvider>
      </ErrorBoundary>
    </ViewportProvider>
  );
}
