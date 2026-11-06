import type { AnalyticsEvent, AnalyticsProvider } from "@/lib/analytics/types";

export interface FakeAnalytics extends AnalyticsProvider {
  readonly events: AnalyticsEvent[];
}

/** Records every tracked event; the privacy harness (Task 0.2.5b) asserts over `events`. */
export function createFakeAnalytics(): FakeAnalytics {
  const events: AnalyticsEvent[] = [];
  return { metadata: { name: "fake" }, events, track: (e) => void events.push(e) };
}
