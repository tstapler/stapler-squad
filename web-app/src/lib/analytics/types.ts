export interface AnalyticsEvent {
  name: string;
  category: "user_action" | "performance" | "navigation" | "rpc";
  durationMs?: number;
  sessionId?: string;
  page?: string;
  component?: string;
  labels?: Record<string, string>;
}

export interface AnalyticsProviderMetadata {
  readonly name: string;
}

export interface AnalyticsProvider {
  readonly metadata: AnalyticsProviderMetadata;
  initialize?(): Promise<void>;
  onClose?(): Promise<void>;
  track(event: AnalyticsEvent): void;
  flush?(): Promise<void>;
}
