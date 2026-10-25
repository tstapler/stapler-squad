// Constants shared by the build stages and verify-output. No Bun APIs: Jest (Node) imports this file.

/** Every NEXT_PUBLIC_* the app reads; build.test.ts derives the list from src/ so a sixth fails the test. */
export const PUBLIC_ENV_VARS = [
  "NEXT_PUBLIC_API_URL",
  "NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING",
  "NEXT_PUBLIC_OTEL_ENABLED",
  "NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
  "NEXT_PUBLIC_RECONNECT_V2",
] as const;

/** Mirrors server/middleware/auth.go exemptPrefixes without the leading slash (a Go test pins the parity in build.test.ts). */
export const EXEMPT_PREFIXES = ["auth/", "login", "health", "_next/", "favicon"] as const;

/** Where Go serves immutable assets from; Bun's defaults would land outside it (ADR-003). */
export const NAMING = {
  entry: "_next/static/chunks/[name]-[hash].[ext]",
  chunk: "_next/static/chunks/[name]-[hash].[ext]",
  asset: "_next/static/media/[name]-[hash].[ext]",
} as const;

export const STAGE_NAMES = ["guard", "clean", "theme-map", "bundle", "html", "public", "verify", "swap"] as const;
export type StageName = (typeof STAGE_NAMES)[number];

export const THEME_BOOT_PLACEHOLDER = "<!--THEME_BOOT-->";
export const FONT_URL_PREFIX = "/_next/static/media/";
export const FONT_PRELOAD_PLACEHOLDER = "<!--FONT_PRELOAD-->";
/** Primary UI font (the default clean theme's sans); preloaded so first paint does not wait for the CSS to discover it. */
export const PRELOAD_FONTS = ["inter-variable.woff2"] as const;
