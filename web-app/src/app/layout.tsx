import type { Metadata, Viewport } from "next";
import { RootShell } from "./RootShell";
import { matrixTheme, cyberpunk77Theme, wh40kTheme, cleanTheme, lightTheme, darkTheme } from "@/styles/theme.css";
import { buildThemeBootScript } from "@/styles/theme-boot-script";
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
  // FOUC-prevention: the map is embedded at build time from the imported vanilla-extract class
  // strings so the inline script can look up the hashed class name before React hydrates.
  const foucScript = buildThemeBootScript({
    matrix: matrixTheme,
    cyberpunk77: cyberpunk77Theme,
    wh40k: wh40kTheme,
    clean: cleanTheme,
    light: lightTheme,
    dark: darkTheme,
  });

  return (
    <html
      lang="en"
      className={`${cleanTheme} ${jetbrainsMono.variable} ${rajdhani.variable} ${cinzel.variable} ${inter.variable}`}
      suppressHydrationWarning
    >
      <head>
        {/* eslint-disable-next-line react/no-danger */}
        <script dangerouslySetInnerHTML={{ __html: foucScript }} />
      </head>
      <body>
        <RootShell>{children}</RootShell>
      </body>
    </html>
  );
}
