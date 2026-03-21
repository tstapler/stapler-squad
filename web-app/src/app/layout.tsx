import type { Metadata, Viewport } from "next";
import { ConditionalHeader } from "@/components/layout/ConditionalHeader";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import { NotificationProvider } from "@/lib/contexts/NotificationContext";
import { OmnibarProvider } from "@/lib/contexts/OmnibarContext";
import { AuthProvider } from "@/lib/contexts/AuthContext";
import { NotificationPanel } from "@/components/ui/NotificationPanel";
import "./globals.css";

export const metadata: Metadata = {
  title: "Stapler Squad Sessions",
  description: "Manage your AI agent sessions",
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  maximumScale: 5,
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>
        <ErrorBoundary>
          <AuthProvider>
            <NotificationProvider>
              <OmnibarProvider>
                <a href="#main-content" className="skip-link">Skip to main content</a>
                <ConditionalHeader />
                {children}
                <NotificationPanel />
              </OmnibarProvider>
            </NotificationProvider>
          </AuthProvider>
        </ErrorBoundary>
      </body>
    </html>
  );
}
