import { FeatureGuard } from "../FeatureGuard";

export default function ChangelogLayout({ children }: { children: React.ReactNode }) {
  return <FeatureGuard flag="changelog">{children}</FeatureGuard>;
}
