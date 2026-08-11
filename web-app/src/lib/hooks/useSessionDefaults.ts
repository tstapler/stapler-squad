"use client";

import { useState, useEffect } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";

export type FieldSource = "global" | "directory" | "profile" | "none";

export interface ResolvedDefaults {
  program: string;
  autoYes: boolean;
  tags: string[];
  envVars: Record<string, string>;
  cliFlags: string;
}

export interface FieldSources {
  program: FieldSource;
  autoYes: FieldSource;
  tags: FieldSource;
  cliFlags: FieldSource;
}

export interface UseSessionDefaultsResult {
  defaults: ResolvedDefaults | null;
  fieldSources: FieldSources;
  loading: boolean;
  error: string | null;
  profiles: string[];
}

const emptyFieldSources: FieldSources = {
  program: "none",
  autoYes: "none",
  tags: "none",
  cliFlags: "none",
};

function deriveFieldSource(
  value: string | boolean | string[],
  usedProfile: boolean,
  usedDirectory: boolean,
  usedGlobal: boolean,
): FieldSource {
  const isEmpty =
    value === "" ||
    value === false ||
    (Array.isArray(value) && value.length === 0);

  if (isEmpty) return "none";
  if (usedProfile) return "profile";
  if (usedDirectory) return "directory";
  if (usedGlobal) return "global";
  return "none";
}

export function useSessionDefaults(
  workingDir: string,
  profileName?: string,
): UseSessionDefaultsResult {
  const [defaults, setDefaults] = useState<ResolvedDefaults | null>(null);
  const [fieldSources, setFieldSources] = useState<FieldSources>(emptyFieldSources);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [profiles, setProfiles] = useState<string[]>([]);

  // Fetch available profiles on mount
  useEffect(() => {
    const fetchProfiles = async () => {
      try {
        const client = createClient(SessionService, getConnectTransport());
        const response = await client.getSessionDefaults({});
        const config = response.defaults;
        if (config?.profiles) {
          setProfiles(Object.keys(config.profiles));
        }
      } catch (err) {
        // Non-critical: profile list fails silently
        console.error("Failed to fetch session defaults config:", err);
      }
    };

    fetchProfiles();
  }, []);

  // Resolve defaults when workingDir or profileName changes
  useEffect(() => {
    if (!workingDir) {
      setDefaults(null);
      setFieldSources(emptyFieldSources);
      setLoading(false);
      setError(null);
      return;
    }

    const fetchDefaults = async () => {
      setLoading(true);
      setError(null);

      try {
        const client = createClient(SessionService, getConnectTransport());
        const response = await client.resolveDefaults({
          workingDir,
          profileName: profileName || "",
        });

        const resolved: ResolvedDefaults = {
          program: response.program,
          autoYes: response.autoYes,
          tags: response.tags,
          envVars: response.envVars ? { ...response.envVars } : {},
          cliFlags: response.cliFlags,
        };

        const sources: FieldSources = {
          program: deriveFieldSource(
            response.program,
            response.usedProfile,
            response.usedDirectory,
            response.usedGlobal,
          ),
          autoYes: deriveFieldSource(
            response.autoYes,
            response.usedProfile,
            response.usedDirectory,
            response.usedGlobal,
          ),
          tags: deriveFieldSource(
            response.tags,
            response.usedProfile,
            response.usedDirectory,
            response.usedGlobal,
          ),
          cliFlags: deriveFieldSource(
            response.cliFlags,
            response.usedProfile,
            response.usedDirectory,
            response.usedGlobal,
          ),
        };

        setDefaults(resolved);
        setFieldSources(sources);
      } catch (err) {
        console.error("Failed to resolve session defaults:", err);
        setError(
          err instanceof Error ? err.message : "Failed to resolve defaults",
        );
        setDefaults(null);
        setFieldSources(emptyFieldSources);
      } finally {
        setLoading(false);
      }
    };

    fetchDefaults();
  }, [workingDir, profileName]);

  return { defaults, fieldSources, loading, error, profiles };
}
