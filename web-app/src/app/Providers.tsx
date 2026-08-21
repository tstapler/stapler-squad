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
import { OnboardingProvider } from "@/lib/contexts/OnboardingContext";

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
                          {children}
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
