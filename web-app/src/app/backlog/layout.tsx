import { FeatureGuard } from "../FeatureGuard";
import { BacklogStreamProvider } from "@/components/backlog/BacklogStreamContext";

export default function BacklogLayout({ children }: { children: React.ReactNode }) {
  return (
    <FeatureGuard flag="backlog">
      <BacklogStreamProvider>{children}</BacklogStreamProvider>
    </FeatureGuard>
  );
}
