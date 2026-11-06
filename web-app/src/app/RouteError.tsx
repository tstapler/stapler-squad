// analytics-exempt
import { ErrorState } from "@/components/ui/ErrorState";

/** The ErrorBoundary's default UI, rendered by the router's errorElement inside `<main>` so the shell survives a failed page. */
export function RouteError({ error, retry }: { error: unknown; retry: () => void }) {
  return (
    <ErrorState
      error={error instanceof Error ? error : null}
      title="Something went wrong"
      message="An unexpected error occurred. Please try again."
      onRetry={retry}
      actionLabel="Retry"
      showDetails={process.env.NODE_ENV === "development"}
    />
  );
}
