// Single owner of WEB_BUILD parsing; launch.ts, test-server.ts and tests/e2e/helpers/web-build.ts import this
// instead of re-reading the variable (plan consistency C8).
export const WEB_BUILD_MODES = ["next", "bun"] as const;
export type WebBuildMode = (typeof WEB_BUILD_MODES)[number];

/** Unset or empty means "next", the default until the cutover; anything else must be an allowed value. */
export function parseWebBuildMode(env: Record<string, string | undefined>): WebBuildMode {
  const raw = env.WEB_BUILD;
  if (raw === undefined || raw === "") return "next";
  if ((WEB_BUILD_MODES as readonly string[]).includes(raw)) return raw as WebBuildMode;
  throw new Error(`WEB_BUILD must be one of ${WEB_BUILD_MODES.join(", ")}; got "${raw}"`);
}
