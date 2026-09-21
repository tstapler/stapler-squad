import { useState, useEffect } from "react";
import { PROGRAMS, type ProgramOption } from "@/lib/constants/programs";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";

export function useAvailablePrograms(): ProgramOption[] {
  const [programs, setPrograms] = useState<ProgramOption[]>(PROGRAMS);

  useEffect(() => {
    // Guard against environments (e.g. tests) where fetch or connect transport is not configured
    if (typeof fetch !== "function") return;

    let mounted = true;

    try {
      const client = createClient(SessionService, getConnectTransport());
      // abort-signal-exempt
      client.listProgramsConfig({})
        .then((res) => {
          if (!mounted) return;
          if (res.programs && res.programs.length > 0) {
            const list: ProgramOption[] = res.programs.map((p) => ({
              value: p.id,
              label: p.label,
              description: p.description || p.command,
            }));
            setPrograms(list);
          }
        })
        .catch(() => {
          // Fallback to /api/server-info if ListProgramsConfig fails
          fetch("/api/server-info")
            .then((r) => r.json())
            .then((data: { programs?: string[] }) => {
              if (!mounted || !Array.isArray(data.programs)) return;
              const staticValues = new Set(PROGRAMS.map((p) => p.value));
              const extra: ProgramOption[] = [];
              for (const fullPath of data.programs) {
                const basename = fullPath.split("/").pop() ?? fullPath;
                if (!staticValues.has(fullPath) && !staticValues.has(basename)) {
                  extra.push({ value: basename, label: basename, description: fullPath });
                }
              }
              if (extra.length > 0) {
                setPrograms([...PROGRAMS, ...extra]);
              }
            })
            .catch(() => {});
        });
    } catch {
      // Fall back silently
    }

    return () => {
      mounted = false;
    };
  }, []);

  return programs;
}
