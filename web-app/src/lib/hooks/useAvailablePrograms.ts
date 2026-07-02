import { useState, useEffect } from "react";
import { PROGRAMS, type ProgramOption } from "@/lib/constants/programs";

export function useAvailablePrograms(): ProgramOption[] {
  const [programs, setPrograms] = useState<ProgramOption[]>(PROGRAMS);

  useEffect(() => {
    // Guard against environments (e.g. tests) where fetch may not be configured
    if (typeof fetch !== "function") return;
    const fetchResult = fetch("/api/server-info");
    if (!fetchResult || typeof fetchResult.then !== "function") return;
    fetchResult
      .then((r) => r.json())
      .then((data: { programs?: string[] }) => {
        if (!Array.isArray(data.programs)) return;

        const staticValues = new Set(PROGRAMS.map((p) => p.value));
        const extra: ProgramOption[] = [];

        for (const fullPath of data.programs) {
          const basename = fullPath.split("/").pop() ?? fullPath;
          // Add if neither the full path nor the basename already appears in the static list
          if (!staticValues.has(fullPath) && !staticValues.has(basename)) {
            extra.push({ value: basename, label: basename, description: fullPath });
          }
        }

        if (extra.length > 0) {
          setPrograms([...PROGRAMS, ...extra]);
        }
      })
      .catch(() => {
        // server-info is not critical; fall back to static list
      });
  }, []);

  return programs;
}
