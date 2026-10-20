import React, { type ReactNode } from "react";
import { createRouterTransport, type ServiceImpl } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { FeatureFlagsProvider } from "@/lib/contexts/FeatureFlagsContext";
import { AnalyticsContextProvider } from "@/lib/contexts/AnalyticsContext";
import { FileTransportProvider } from "@/lib/hooks/useFileTransport";
import type { FileTransportPort } from "@/lib/files/transport/fileTransport";
import { fakeTransportRef } from "./mockTransport";
import { createFakeAnalytics, type FakeAnalytics } from "./fakeAnalytics";

export type SessionHandlers = Partial<ServiceImpl<typeof SessionService>>;

export interface FilesHarnessOverrides {
  sessionId?: string;
  /** Flag name -> enabled, served by GetFeatureFlags. */
  flags?: Record<string, boolean>;
  /** Replaces or adds SessionService RPC handlers (e.g. getFileContent). */
  handlers?: SessionHandlers;
  /** FileTransport the hooks under test receive; defaults to the production router over the fake Connect transport. */
  transport?: FileTransportPort;
}

export interface FilesHarness {
  sessionId: string;
  analytics: FakeAnalytics;
  /** Names of the RPCs the unit under test called, in order. */
  calls: string[];
  wrapper: (props: { children: ReactNode }) => React.JSX.Element;
}

/** One factory for the fake Connect transport, flags + analytics providers and the session id. */
export function createFilesTestHarness(overrides: FilesHarnessOverrides = {}): FilesHarness {
  const calls: string[] = [];
  const analytics = createFakeAnalytics();
  const flags = Object.entries(overrides.flags ?? {}).map(([name, enabled]) => ({ name, enabled }));
  const handlers: SessionHandlers = { getFeatureFlags: () => ({ flags }), ...overrides.handlers };
  const traced = Object.fromEntries(
    Object.entries(handlers).map(([rpc, fn]) => [
      rpc,
      (...args: unknown[]) => {
        calls.push(rpc);
        return (fn as (...a: unknown[]) => unknown)(...args);
      },
    ]),
  ) as SessionHandlers;
  fakeTransportRef.current = createRouterTransport(({ service }) => {
    service(SessionService, traced);
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <AnalyticsContextProvider provider={analytics}>
      <FeatureFlagsProvider>
        <FileTransportProvider value={overrides.transport}>{children}</FileTransportProvider>
      </FeatureFlagsProvider>
    </AnalyticsContextProvider>
  );
  return { sessionId: overrides.sessionId ?? "session-1", analytics, calls, wrapper };
}
