import type { Metadata, Viewport } from "next";
import { ConditionalHeader } from "@/components/layout/ConditionalHeader";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import { AuthProvider } from "@/lib/contexts/AuthContext";
import { Providers } from "./Providers";
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
            <Providers>
              <a href="#main-content" className="skip-link">Skip to main content</a>
              <ConditionalHeader />
              {children}
              <NotificationPanel />
            </Providers>
          </AuthProvider>
        </ErrorBoundary>
      </body>
    </html>
  );
}
