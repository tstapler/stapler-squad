"use client";

import { useRef } from "react";
import { Provider } from "react-redux";
import { store } from "@/lib/store/store";
import { NotificationProvider } from "@/lib/contexts/NotificationContext";
import { OmnibarProvider } from "@/lib/contexts/OmnibarContext";
import { ReviewQueueProvider } from "@/lib/contexts/ReviewQueueContext";
import { ApprovalsProvider } from "@/lib/contexts/ApprovalsContext";
import { GlobalSessionServiceProvider } from "@/lib/contexts/SessionServiceContext";
import { NavigationProvider } from "@/lib/contexts/NavigationContext";
import { ThemeProvider } from "@/lib/contexts/ThemeContext";
import { AnalyticsContextProvider } from "@/lib/contexts/AnalyticsContext";
import { HttpAnalyticsProvider } from "@/lib/analytics/HttpAnalyticsProvider";
import { ConsoleAnalyticsProvider } from "@/lib/analytics/ConsoleAnalyticsProvider";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";
import { WebVitalsReporter } from "@/components/telemetry/WebVitalsReporter";

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
          <NavigationProvider>
            <NotificationProvider>
              <GlobalSessionServiceProvider>
                <OmnibarProvider>
                  <ReviewQueueProvider>
                    <ApprovalsProvider>
                      {children}
                    </ApprovalsProvider>
                  </ReviewQueueProvider>
                </OmnibarProvider>
              </GlobalSessionServiceProvider>
            </NotificationProvider>
          </NavigationProvider>
        </ThemeProvider>
      </Provider>
    </AnalyticsContextProvider>
  );
}
