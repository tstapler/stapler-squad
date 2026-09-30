"use client";

import { useRef } from "react";
import { Provider } from "react-redux";
import { store } from "@/lib/store/store";
import { NotificationProvider } from "@/lib/contexts/NotificationContext";
import { OmnibarProvider } from "@/lib/contexts/OmnibarContext";
import { ReviewQueueProvider } from "@/lib/contexts/ReviewQueueContext";
import { ApprovalsProvider } from "@/lib/contexts/ApprovalsContext";
import { StuckBacklogItemsProvider } from "@/lib/hooks/useStuckBacklogItems";
import { GlobalSessionServiceProvider } from "@/lib/contexts/SessionServiceContext";
import { SystemMemoryProvider } from "@/lib/contexts/SystemMemoryContext";
import { NavigationProvider } from "@/lib/contexts/NavigationContext";
import { ThemeProvider } from "@/lib/contexts/ThemeContext";
import { AnalyticsContextProvider } from "@/lib/contexts/AnalyticsContext";
import { FeatureFlagsProvider } from "@/lib/contexts/FeatureFlagsContext";
import { HttpAnalyticsProvider } from "@/lib/analytics/HttpAnalyticsProvider";
import { ConsoleAnalyticsProvider } from "@/lib/analytics/ConsoleAnalyticsProvider";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";
import { WebVitalsReporter } from "@/components/telemetry/WebVitalsReporter";
import { OtelInit } from "@/components/telemetry/OtelInit";
import { OnboardingProvider } from "@/lib/contexts/OnboardingContext";
import { TerminalPoolProvider, DEFAULT_TERMINAL_POOL_MAX_SIZE } from "@/lib/terminal/TerminalPool";

export function Providers({ children }: { children: React.ReactNode }) {
  // Create provider once per mount using a ref so the instance is stable.
  // This must live in a "use client" component — class instances cannot cross
  // the server/client boundary as serialized props.
  const analyticsProviderRef = useRef(
    process.env.NODE_ENV === "production"
      ? new HttpAnalyticsProvider()
      : new ConsoleAnalyticsProvider()
  );

  return (
    <AnalyticsContextProvider provider={analyticsProviderRef.current}>
      <OtelInit />
      <WebVitalsReporter />
      <PageViewTracker />
      <Provider store={store}>
        <ThemeProvider>
          <FeatureFlagsProvider>
          <NavigationProvider>
            <NotificationProvider>
              <GlobalSessionServiceProvider>
                <SystemMemoryProvider>
                <OmnibarProvider>
                  <OnboardingProvider>
                    <ReviewQueueProvider>
                      <ApprovalsProvider>
                        <StuckBacklogItemsProvider>
                          {/* Story 3 (Task 3.4) — must live above PaneSplitRenderer's
                              `key={pane.id}-{pane.sessionId}` remount boundary (see
                              TerminalPool.tsx's module doc comment) so pooled terminal
                              instances survive a pane's assigned session changing. */}
                          <TerminalPoolProvider maxSize={DEFAULT_TERMINAL_POOL_MAX_SIZE}>
                            {children}
                          </TerminalPoolProvider>
                        </StuckBacklogItemsProvider>
                      </ApprovalsProvider>
                    </ReviewQueueProvider>
                  </OnboardingProvider>
                </OmnibarProvider>
                </SystemMemoryProvider>
              </GlobalSessionServiceProvider>
            </NotificationProvider>
          </NavigationProvider>
          </FeatureFlagsProvider>
        </ThemeProvider>
      </Provider>
    </AnalyticsContextProvider>
  );
}
