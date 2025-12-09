"use client";

import { useState, useEffect } from "react";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_connect";

interface BranchSuggestionsOptions {
  repositoryPath?: string;
  baseUrl?: string;
}

/**
 * Hook to provide git branch suggestions based on existing sessions.
 * Returns a list of unique branch names from all sessions, optionally filtered by repository.
 */
export function useBranchSuggestions(options: BranchSuggestionsOptions = {}) {
  const { baseUrl = "http://localhost:8543/api" } = options;
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const fetchSuggestions = async () => {
      try {
        setIsLoading(true);

        // Create ConnectRPC client
        const transport = createConnectTransport({ baseUrl });
        const client = createPromiseClient(SessionService, transport);

        // Fetch all sessions to extract branch names
        const response = await client.listSessions({});
        const sessions = response.sessions || [];

        // Extract unique branch names, optionally filtered by repository
        const branches = new Set<string>();
        sessions.forEach((session) => {
          if (session.branch) {
            // If repository path is specified, only include branches from that repo
            if (options.repositoryPath) {
              if (session.path === options.repositoryPath) {
                branches.add(session.branch);
              }
            } else {
              branches.add(session.branch);
            }
          }
        });

        // Convert to sorted array
        let sortedBranches = Array.from(branches).sort();

        // Add common branch patterns if no suggestions exist
        if (sortedBranches.length === 0) {
          sortedBranches = [
            "main",
            "master",
            "develop",
            "feature/",
            "bugfix/",
            "hotfix/",
            "release/",
          ];
        }

        setSuggestions(sortedBranches);
      } catch (error) {
        console.error("Failed to fetch branch suggestions:", error);
        // Provide fallback suggestions on error
        setSuggestions([
          "main",
          "master",
          "develop",
          "feature/",
          "bugfix/",
        ]);
      } finally {
        setIsLoading(false);
      }
    };

    fetchSuggestions();
  }, [options.repositoryPath, baseUrl]);

  return { suggestions, isLoading };
}
