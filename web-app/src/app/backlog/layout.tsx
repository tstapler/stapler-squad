import { FeatureGuard } from "../FeatureGuard";

export default function BacklogLayout({ children }: { children: React.ReactNode }) {
  return <FeatureGuard flag="backlog">{children}</FeatureGuard>;
}
