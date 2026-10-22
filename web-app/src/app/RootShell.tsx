import type { ReactNode } from "react";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import { AuthProvider } from "@/lib/contexts/AuthContext";
import { Providers } from "./Providers";
import { NotificationPanel } from "@/components/ui/NotificationPanel";
import { TrayErrorBoundary } from "@/components/ui/TrayErrorBoundary";
import { TmuxVersionMismatchBanner } from "@/components/system/TmuxVersionMismatchBanner";
import { ForkPressureStatusBanner } from "@/components/system/ForkPressureStatusBanner";
import { ViewportProvider } from "@/components/providers/ViewportProvider";
import { CockpitShell } from "@/components/layout/CockpitShell";
import { mainContent } from "./layout.css";

/** Provider and chrome tree shared by every entry point (the Next layout now, the Bun entry later). */
export function RootShell({ children }: { children: ReactNode }) {
  return (
    <ViewportProvider>
      <ErrorBoundary>
        <AuthProvider>
          <Providers>
            <CockpitShell>
              <a href="#main-content" className="skip-link">Skip to main content</a>
              <main id="main-content" className={mainContent}>
                <TmuxVersionMismatchBanner />
                <ForkPressureStatusBanner />
                {children}
              </main>
              <TrayErrorBoundary>
                <NotificationPanel />
              </TrayErrorBoundary>
            </CockpitShell>
          </Providers>
        </AuthProvider>
      </ErrorBoundary>
    </ViewportProvider>
  );
}
