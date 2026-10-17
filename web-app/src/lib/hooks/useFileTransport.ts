"use client";

import { createContext, createElement, useContext, useMemo, type ReactNode } from "react";
import { createClient } from "@connectrpc/connect";
import { FileOp, FileWire, SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { useFilesAnalytics } from "@/lib/files/analytics/filesAnalytics";
import { createFullFileTransport } from "@/lib/files/transport/fullFileTransport";
import { FileTransportRouter } from "@/lib/files/transport/router";
import type { AdvertisedOp, FileCapabilities, FileTransportPort, TransportId } from "@/lib/files/transport/fileTransport";

const FileTransportContext = createContext<FileTransportPort | null>(null);

const OPS: Partial<Record<FileOp, AdvertisedOp>> = {
  [FileOp.STAT]: "stat", [FileOp.READ_ALL]: "read_all", [FileOp.READ_LINES]: "read_lines", [FileOp.READ_BYTES]: "read_bytes",
  [FileOp.READ_STREAM]: "read_stream", [FileOp.PATCH]: "patch", [FileOp.WRITE_FULL]: "write_full",
};
const WIRES: Partial<Record<FileWire, TransportId>> = {
  [FileWire.CONNECT_UNARY]: "connect_unary", [FileWire.CONNECT_STREAM]: "connect_stream",
  [FileWire.WS_STREAM]: "ws_stream", [FileWire.HTTP_RANGE]: "http_range",
};

const defined = <T,>(x: T | undefined): x is T => x !== undefined;

/** Production router: the `full` floor plus the capability RPC; higher rungs register as adapters land. */
export function createDefaultFileTransport(analytics?: Pick<ReturnType<typeof useFilesAnalytics>, "emit">): FileTransportPort {
  const client = createClient(SessionService, getConnectTransport());
  return new FileTransportRouter({
    full: createFullFileTransport(client),
    analytics,
    async fetchCapabilities(sessionId): Promise<FileCapabilities> {
      // One cached page-lifetime fetch with no owning component to cancel it.
      // abort-signal-exempt
      const { capabilities: c } = await client.getFileTransportCapabilities({ sessionId });
      const l = c?.limits;
      return {
        protocolVersion: c?.protocolVersion ?? 0,
        features: c?.features ?? [],
        ops: (c?.ops ?? []).map((o) => OPS[o]).filter(defined),
        wires: (c?.wires ?? []).map((w) => WIRES[w]).filter(defined),
        capsEpoch: Number(c?.capsEpoch ?? 0),
        limits: {
          maxRangeBytes: Number(l?.maxRangeBytes ?? 0), maxRangeLines: l?.maxRangeLines ?? 0, preferredChunkBytes: l?.preferredChunkBytes ?? 0,
          maxPatchOps: l?.maxPatchOps ?? 0, maxPatchBodyBytes: Number(l?.maxPatchBodyBytes ?? 0), maxInsertBytes: Number(l?.maxInsertBytes ?? 0),
          maxWriteFullBytes: Number(l?.maxWriteFullBytes ?? 0), hashCapBytes: Number(l?.hashCapBytes ?? 0), maxConcurrentRanges: l?.maxConcurrentRanges ?? 0,
        },
      };
    },
  });
}

/** One router per mounted Files tab: capability cache and demotion state live and die with it. */
export function FileTransportProvider({ value, children }: { value?: FileTransportPort; children: ReactNode }) {
  const analytics = useFilesAnalytics({ component: "FileTransport" });
  const router = useMemo(() => value ?? createDefaultFileTransport(analytics), [value, analytics]);
  return createElement(FileTransportContext.Provider, { value: router }, children);
}

/** The port every Files hook uses; throws outside a provider so a missing wrapper fails loudly. */
export function useFileTransport(): FileTransportPort {
  const transport = useContext(FileTransportContext);
  if (!transport) throw new Error("useFileTransport must be used within a FileTransportProvider");
  return transport;
}
