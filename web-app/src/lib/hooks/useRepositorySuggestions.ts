"use client";

import { useState, useEffect } from "react";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_connect";

interface RepositorySuggestionsOptions {
  baseUrl?: string;
}

/**
 * Hook to provide repository path suggestions based on existing sessions.
 * Returns a list of unique repository paths from all sessions.
 */
export function useRepositorySuggestions(options: RepositorySuggestionsOptions = {}) {
  const { baseUrl = "http://localhost:8543" } = options;
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const fetchSuggestions = async () => {
      try {
        setIsLoading(true);

        // Create ConnectRPC client
        const transport = createConnectTransport({ baseUrl });
        const client = createPromiseClient(SessionService, transport);

        // Fetch all sessions to extract repository paths
        const response = await client.listSessions({});
        const sessions = response.sessions || [];

        // Extract unique repository paths
        const paths = new Set<string>();
        sessions.forEach((session) => {
          if (session.path) {
            paths.add(session.path);
          }
        });

        // Convert to sorted array
        const sortedPaths = Array.from(paths).sort();

        // Add common project directory patterns if no suggestions exist
        if (sortedPaths.length === 0) {
          sortedPaths.push(
            "/Users/username/projects",
            "/Users/username/code",
            "/Users/username/workspace",
            "/Users/username/IdeaProjects"
          );
        }

        setSuggestions(sortedPaths);
      } catch (error) {
        console.error("Failed to fetch repository suggestions:", error);
        // Provide fallback suggestions on error
        setSuggestions([
          "/Users/username/projects",
          "/Users/username/code",
          "/Users/username/workspace",
        ]);
      } finally {
        setIsLoading(false);
      }
    };

    fetchSuggestions();
  }, [baseUrl]);

  return { suggestions, isLoading };
}
