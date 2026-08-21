"use client";

import { useState, useEffect } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { rankPathsByFrecency } from "@/lib/utils/frecency";

/** Returns OS-appropriate example paths shown when no sessions exist yet. */
function getFallbackHints(): string[] {
  if (typeof window === "undefined") return ["/home/username/projects", "/home/username/code"];
  // Prefer the non-deprecated userAgentData API; fall back to platform string.
  const platform =
    (navigator as Navigator & { userAgentData?: { platform?: string } }).userAgentData?.platform ??
    navigator.platform ??
    "";
  const home = platform.toLowerCase().includes("mac") ? "/Users" : "/home";
  return [`${home}/username/projects`, `${home}/username/code`];
}

interface RepositorySuggestionsOptions {
  baseUrl?: string;
}

/**
 * Hook to provide repository path suggestions based on existing sessions.
 * Paths are ranked by frecency (frequency × recency decay) so the project
 * you use most often AND most recently appears first.
 */
export function useRepositorySuggestions(options: RepositorySuggestionsOptions = {}) {
  const {} = options;
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const fetchSuggestions = async () => {
      try {
        setIsLoading(true);

        const client = createClient(SessionService, getConnectTransport());

        const response = await client.listSessions({});
        const sessions = response.sessions || [];

        // Build the input shape rankPathsByFrecency expects
        const frecencyInput = sessions.map((session) => ({
          path: session.path,
          timestampsMs: [
            session.updatedAt          ? Number(session.updatedAt.seconds) * 1000          : 0,
            session.lastMeaningfulOutput ? Number(session.lastMeaningfulOutput.seconds) * 1000 : 0,
            session.createdAt          ? Number(session.createdAt.seconds) * 1000          : 0,
          ],
        }));

        const ranked = rankPathsByFrecency(frecencyInput);

        const fallback = ranked.length === 0 ? getFallbackHints() : null;
        setSuggestions(fallback ?? ranked);
      } catch (error) {
        console.error("Failed to fetch repository suggestions:", error);
        setSuggestions(getFallbackHints());
      } finally {
        setIsLoading(false);
      }
    };

    fetchSuggestions();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { suggestions, isLoading };
}
