// analytics-exempt
import { Skeleton } from "@/components/ui/Skeleton";
import { container } from "./RoutePending.css";

/** Route-chunk skeleton. It stays silent: busyness is `aria-busy` on the layout main, and the route announcer speaks once. */
export function RoutePending() {
  return (
    <div className={container} data-testid="route-pending">
      <Skeleton variant="text" width="30%" height={28} />
      <Skeleton width="100%" height={96} />
      <Skeleton width="100%" height={96} />
    </div>
  );
}
