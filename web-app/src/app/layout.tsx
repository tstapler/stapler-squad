import type { Metadata, Viewport } from "next";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import { AuthProvider } from "@/lib/contexts/AuthContext";
import { Providers } from "./Providers";
import { NotificationPanel } from "@/components/ui/NotificationPanel";
import { ViewportProvider } from "@/components/providers/ViewportProvider";
import { WebVitalsReporter } from "@/components/telemetry/WebVitalsReporter";
import { CockpitShell } from "@/components/layout/CockpitShell";
import { matrixTheme, cyberpunk77Theme, wh40kTheme, cleanTheme, lightTheme, darkTheme } from "@/styles/theme.css";
import { jetbrainsMono, rajdhani, cinzel, inter } from "./fonts";
import "./globals.css";
import "@/styles/globalEffects.css";

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
  // FOUC-prevention: the themeMap is embedded at build time from the imported
  // vanilla-extract class strings so the inline script can look up the correct
  // hashed class name before React hydrates.
  const themeMapJson = JSON.stringify({
    matrix: matrixTheme,
    cyberpunk77: cyberpunk77Theme,
    wh40k: wh40kTheme,
    clean: cleanTheme,
    light: lightTheme,
    dark: darkTheme,
  });

  const foucScript = `(function(){try{var m=${themeMapJson};var t=localStorage.getItem('stapler-theme');var cls=t&&m[t]?m[t]:m['matrix'];document.documentElement.className=document.documentElement.className.replace(/\\S+/g,'').trim();document.documentElement.classList.add(cls);}catch(e){}})();`;

  return (
    <html
      lang="en"
      className={`${matrixTheme} ${jetbrainsMono.variable} ${rajdhani.variable} ${cinzel.variable} ${inter.variable}`}
      suppressHydrationWarning
    >
      <head>
        {/* eslint-disable-next-line react/no-danger */}
        <script dangerouslySetInnerHTML={{ __html: foucScript }} />
      </head>
      <body>
        <WebVitalsReporter />
        <ViewportProvider>
          <ErrorBoundary>
            <AuthProvider>
              <Providers>
                <CockpitShell>
                  <a href="#main-content" className="skip-link" style={{ position: "absolute", left: "-9999px" }}>Skip to main content</a>
                  <main id="main-content" style={{ flex: 1, overflow: "hidden", display: "flex", flexDirection: "column" }}>
                    {children}
                  </main>
                  <NotificationPanel />
                </CockpitShell>
              </Providers>
            </AuthProvider>
          </ErrorBoundary>
        </ViewportProvider>
      </body>
    </html>
  );
}
