"use client";

// +feature: ui:bundle-export

import { useCallback, useEffect, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import {
  SessionBundleService,
  ExportSessionBundleRequestSchema,
  SaveSessionBundleRequestSchema,
} from "@/gen/session/v1/rum_bundle_pb";
import type { SaveSessionBundleResponse } from "@/gen/session/v1/rum_bundle_pb";
import { create } from "@bufbuild/protobuf";
import { getConnectTransport } from "@/lib/api/transport";

export interface UseExportBundleReturn {
  download: (sessionUuid: string, sessionTitle: string) => Promise<boolean>;
  saveOnServer: (sessionUuid: string) => Promise<SaveSessionBundleResponse>;
  loading: boolean;
  error: string | null;
}

const ZIP_MIME_TYPE = "application/zip";

function slugifyTitle(title: string): string {
  return title.toLowerCase().replace(/\s+/g, "-").replace(/[^a-z0-9-]/g, "");
}

function formatTimestamp(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  const ymd = `${date.getFullYear()}${pad(date.getMonth() + 1)}${pad(date.getDate())}`;
  const hms = `${pad(date.getHours())}${pad(date.getMinutes())}${pad(date.getSeconds())}`;
  return `${ymd}-${hms}`;
}

function buildFilename(sessionTitle: string): string {
  return `${slugifyTitle(sessionTitle)}-${formatTimestamp(new Date())}.zip`;
}

function triggerBrowserDownload(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

async function deliverBlob(blob: Blob, filename: string): Promise<void> {
  const file = new File([blob], filename, { type: ZIP_MIME_TYPE });
  if (typeof navigator !== "undefined" && navigator.canShare?.({ files: [file] })) {
    try {
      await navigator.share({ files: [file] });
    } catch (err: unknown) {
      const name = err instanceof DOMException ? err.name : "";
      if (name === "AbortError") return; // user dismissed the share sheet
      if (name !== "NotAllowedError") throw err;
      triggerBrowserDownload(blob, filename); // user activation expired during the stream
    }
  } else {
    triggerBrowserDownload(blob, filename);
  }
}

type BundleClient = ReturnType<typeof createClient<typeof SessionBundleService>>;

// kibitzer:ignore long-function -- standard React hook: two useCallback handlers + setup boilerplate cannot be split further without sacrificing readability
export function useExportBundle(): UseExportBundleReturn {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Initialize client inside useEffect to avoid SSR issues (matching useExportRules pattern).
  const clientRef = useRef<BundleClient | null>(null);

  useEffect(() => {
    clientRef.current = createClient(SessionBundleService, getConnectTransport());
  }, []);

  const download = useCallback(async (sessionUuid: string, sessionTitle: string) => {
    if (!clientRef.current) return false;
    setLoading(true);
    setError(null);
    try {
      const req = create(ExportSessionBundleRequestSchema, {
        sessionUuid,
        maxScrollbackLines: 0, // server default (500 lines)
      });
      const chunks: Uint8Array<ArrayBuffer>[] = [];
      for await (const chunk of clientRef.current.exportSessionBundle(req)) {
        if (chunk.data && chunk.data.length > 0) {
          chunks.push(new Uint8Array(chunk.data));
        }
      }
      const blob = new Blob(chunks, { type: ZIP_MIME_TYPE });
      await deliverBlob(blob, buildFilename(sessionTitle));
      return true;
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Export failed");
      return false;
    } finally {
      setLoading(false);
    }
  }, []);

  const saveOnServer = useCallback(async (sessionUuid: string): Promise<SaveSessionBundleResponse> => {
    if (!clientRef.current) throw new Error("Client not initialized");
    setLoading(true);
    setError(null);
    try {
      const req = create(SaveSessionBundleRequestSchema, { sessionUuid, outputDir: "" });
      return await clientRef.current.saveSessionBundle(req);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Save failed");
      throw err;
    } finally {
      setLoading(false);
    }
  }, []);

  return { download, saveOnServer, loading, error };
}
