import { FeatureGuard } from "../../FeatureGuard";

export default function BacklogSourcesSettingsLayout({ children }: { children: React.ReactNode }) {
  return <FeatureGuard flag="backlog">{children}</FeatureGuard>;
}
