import { FeatureGuard } from "../../FeatureGuard";

// Rendered (not `metadata`) so React hoists a <title> into the static <head>: Next streams
// metadata behind the client FeatureGuard, leaving the exported HTML without one. Must equal the
// page's metadata title and the route-meta.ts row.
const TITLE = "Backlog Sources - Settings - Stapler Squad";

export default function BacklogSourcesSettingsLayout({ children }: { children: React.ReactNode }) {
  return (
    <>
      <title>{TITLE}</title>
      <FeatureGuard flag="backlog">{children}</FeatureGuard>
    </>
  );
}
