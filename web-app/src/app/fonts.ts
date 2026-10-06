import localFont from "next/font/local";

// Fonts are vendored under ./fonts/ (SIL Open Font License 1.1 — see the
// OFL.txt alongside each family) instead of using next/font/google, which
// fetches from fonts.gstatic.com at build time and made CI flaky on network
// hiccups.

export const jetbrainsMono = localFont({
  src: [
    {
      path: "./fonts/jetbrains-mono/jetbrains-mono-variable-normal.woff2",
      style: "normal",
    },
    {
      path: "./fonts/jetbrains-mono/jetbrains-mono-variable-italic.woff2",
      style: "italic",
    },
  ],
  variable: "--font-jetbrains-mono",
});

export const rajdhani = localFont({
  src: [
    { path: "./fonts/rajdhani/rajdhani-400.woff2", weight: "400", style: "normal" },
    { path: "./fonts/rajdhani/rajdhani-500.woff2", weight: "500", style: "normal" },
    { path: "./fonts/rajdhani/rajdhani-600.woff2", weight: "600", style: "normal" },
    { path: "./fonts/rajdhani/rajdhani-700.woff2", weight: "700", style: "normal" },
  ],
  variable: "--font-rajdhani",
});

export const cinzel = localFont({
  // Google serves the same variable-font file for both static weight requests;
  // the browser uses the file's weight axis to render 400 vs 700.
  src: [
    { path: "./fonts/cinzel/cinzel-variable.woff2", weight: "400", style: "normal" },
    { path: "./fonts/cinzel/cinzel-variable.woff2", weight: "700", style: "normal" },
  ],
  variable: "--font-cinzel",
});

export const inter = localFont({
  src: "./fonts/inter/inter-variable.woff2",
  variable: "--font-inter",
});
