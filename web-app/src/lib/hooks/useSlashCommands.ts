import { useState, useEffect, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { SlashCommandInfo } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";

export type { SlashCommandInfo };

interface UseSlashCommandsResult {
  commands: SlashCommandInfo[];
  isLoading: boolean;
}

/**
 * Fetches available slash commands for a given target directory.
 * Merges project (.claude/commands/), user (~/.claude/commands/), and built-in commands.
 * Re-fetches whenever targetDirectory changes.
 */
export function useSlashCommands(targetDirectory: string): UseSlashCommandsResult {
  const [commands, setCommands] = useState<SlashCommandInfo[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    abortRef.current?.abort();
    const abort = new AbortController();
    abortRef.current = abort;

    setIsLoading(true);

    const client = createClient(SessionService, getConnectTransport());
    client
      .listSlashCommands({ targetDirectory }, { signal: abort.signal })
      .then((resp) => {
        if (!abort.signal.aborted) {
          setCommands(resp.commands);
        }
      })
      .catch(() => {
        /* ignore abort errors */
      })
      .finally(() => {
        if (!abort.signal.aborted) {
          setIsLoading(false);
        }
      });

    return () => {
      abort.abort();
    };
  }, [targetDirectory]);

  return { commands, isLoading };
}
