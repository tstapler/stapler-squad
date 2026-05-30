"use client";
// +feature: shell-tabs

import { useState, useEffect, useCallback, useRef, useContext } from "react";
import { Shell, ShellStatus } from "@/gen/session/v1/types_pb";
import { SpawnShellRequest } from "@/gen/session/v1/session_pb";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { createWatchTransport } from "@/lib/transport/watch-ws-transport";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { createRpcTimingInterceptor } from "@/lib/telemetry/rpcTiming";
import { AnalyticsContext } from "@/lib/contexts/AnalyticsContext";

export type ShellTab = {
  id: string;
  name: string;
  command: string;
  status: "running" | "stopped" | "error";
  exitCode?: number;
};

function shellToTab(shell: Shell): ShellTab {
  let status: ShellTab["status"] = "running";
  if (shell.status === ShellStatus.STOPPED) status = "stopped";
  else if (shell.status === ShellStatus.ERROR) status = "error";

  return {
    id: shell.id,
    name: shell.name || shell.command || "shell",
    command: shell.command,
    status,
    exitCode: shell.exitCode !== 0 ? shell.exitCode : undefined,
  };
}

interface UseShellsReturn {
  shells: ShellTab[];
  isLoading: boolean;
  spawnShell: (req: { name?: string; command?: string; workingDir?: string }) => Promise<ShellTab | null>;
  stopShell: (shellId: string) => Promise<boolean>;
  restartShell: (shellId: string) => Promise<boolean>;
  deleteShell: (shellId: string) => Promise<boolean>;
  updateShellStatus: (shellId: string, status: ShellTab["status"], exitCode?: number) => void;
  refetch: () => void;
}

export function useShells(sessionId: string): UseShellsReturn {
  const [shells, setShells] = useState<ShellTab[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  // Use context directly so useShells works in test environments without AnalyticsContextProvider
  const analyticsCtx = useContext(AnalyticsContext);
  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  useEffect(() => {
    const interceptors = analyticsCtx
      ? [createAuthInterceptor(), createRpcTimingInterceptor(analyticsCtx)]
      : [createAuthInterceptor()];
    const transport = createWatchTransport({
      baseUrl: getApiBaseUrl(),
      interceptors,
    });
    clientRef.current = createClient(SessionService, transport);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const refetch = useCallback(async () => {
    if (!clientRef.current || !sessionId) return;
    setIsLoading(true);
    try {
      const response = await clientRef.current.listShells({ sessionId });
      setShells(response.shells.map(shellToTab));
    } catch {
      // Silently ignore — shells are additive
    } finally {
      setIsLoading(false);
    }
  }, [sessionId]);

  useEffect(() => {
    refetch();
  }, [refetch]);

  const spawnShell = useCallback(
    async (req: { name?: string; command?: string; workingDir?: string }): Promise<ShellTab | null> => {
      if (!clientRef.current || !sessionId) return null;
      try {
        const response = await clientRef.current.spawnShell({
          sessionId,
          name: req.name ?? "",
          command: req.command ?? "",
          workingDir: req.workingDir ?? "",
        } as SpawnShellRequest);
        if (!response.shell) return null;
        const tab = shellToTab(response.shell);
        setShells(prev => [...prev, tab]);
        return tab;
      } catch (err) {
        throw err;
      }
    },
    [sessionId]
  );

  const stopShell = useCallback(
    async (shellId: string): Promise<boolean> => {
      if (!clientRef.current || !sessionId) return false;
      try {
        const response = await clientRef.current.stopShell({ sessionId, shellId });
        if (response.success) {
          setShells(prev =>
            prev.map(s => s.id === shellId ? { ...s, status: "stopped" as const } : s)
          );
        }
        return response.success;
      } catch {
        return false;
      }
    },
    [sessionId]
  );

  const restartShell = useCallback(
    async (shellId: string): Promise<boolean> => {
      if (!clientRef.current || !sessionId) return false;
      try {
        const response = await clientRef.current.restartShell({ sessionId, shellId });
        if (response.success) {
          setShells(prev =>
            prev.map(s => s.id === shellId ? { ...s, status: "running" as const, exitCode: undefined } : s)
          );
        }
        return response.success;
      } catch {
        return false;
      }
    },
    [sessionId]
  );

  const deleteShell = useCallback(
    async (shellId: string): Promise<boolean> => {
      if (!clientRef.current || !sessionId) return false;
      try {
        const response = await clientRef.current.deleteShell({ sessionId, shellId });
        if (response.success) {
          setShells(prev => prev.filter(s => s.id !== shellId));
        }
        return response.success;
      } catch {
        return false;
      }
    },
    [sessionId]
  );

  const updateShellStatus = useCallback(
    (shellId: string, status: ShellTab["status"], exitCode?: number) => {
      setShells(prev =>
        prev.map(s =>
          s.id === shellId
            ? { ...s, status, exitCode: exitCode !== undefined ? exitCode : s.exitCode }
            : s
        )
      );
    },
    []
  );

  return { shells, isLoading, spawnShell, stopShell, restartShell, deleteShell, updateShellStatus, refetch };
}
