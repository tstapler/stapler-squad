import type { Metadata } from "next";
import { Header } from "@/components/layout/Header";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import { NotificationProvider } from "@/lib/contexts/NotificationContext";
import { NotificationPanel } from "@/components/ui/NotificationPanel";
import "./globals.css";

export const metadata: Metadata = {
  title: "Claude Squad Sessions",
  description: "Manage your AI agent sessions",
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
          <NotificationProvider>
            <Header />
            {children}
            <NotificationPanel />
          </NotificationProvider>
        </ErrorBoundary>
      </body>
    </html>
  );
}
