import type { Metadata, Viewport } from "next";
import { ConditionalHeader } from "@/components/layout/ConditionalHeader";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import { AuthProvider } from "@/lib/contexts/AuthContext";
import { Providers } from "./Providers";
import { NotificationPanel } from "@/components/ui/NotificationPanel";
import { ViewportProvider } from "@/components/providers/ViewportProvider";
import { ThemeProvider } from "@/components/providers/ThemeProvider";
import { WebVitalsReporter } from "@/components/telemetry/WebVitalsReporter";
import { lightTheme } from "@/styles/theme.css";
import "./globals.css";

export const metadata: Metadata = {
  title: "Stapler Squad Sessions",
  description: "Manage your AI agent sessions",
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  maximumScale: 5,
  viewportFit: "cover",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className={lightTheme} suppressHydrationWarning>
      <body>
        <ThemeProvider />
        <WebVitalsReporter />
        <ViewportProvider>
          <ErrorBoundary>
            <AuthProvider>
              <Providers>
                <a href="#main-content" className="skip-link">Skip to main content</a>
                <ConditionalHeader />
                {children}
                <NotificationPanel />
              </Providers>
            </AuthProvider>
          </ErrorBoundary>
        </ViewportProvider>
      </body>
    </html>
  );
}
