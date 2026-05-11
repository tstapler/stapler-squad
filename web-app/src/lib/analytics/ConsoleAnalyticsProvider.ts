import type { AnalyticsEvent, AnalyticsProvider, AnalyticsProviderMetadata } from "./types";

export class ConsoleAnalyticsProvider implements AnalyticsProvider {
  readonly metadata: AnalyticsProviderMetadata = { name: "ConsoleAnalyticsProvider" };

  track(event: AnalyticsEvent): void {
    console.debug("[analytics]", event.name, event);
  }
}
