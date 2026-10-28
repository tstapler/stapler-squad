import { useState, useEffect } from "react";
import { PROGRAMS, type ProgramOption } from "@/lib/constants/programs";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";

const CACHE_TTL_MS = 30_000;

// One fetch per document: ~58 row/header/detail consumers share this cache and promise.
let cached: { at: number; programs: ProgramOption[] } | null = null;
let inFlight: Flight | null = null;

// The flight is shared, so it aborts only when its last subscriber unmounts.
interface Flight {
  promise: Promise<ProgramOption[]>;
  controller: AbortController;
  subscribers: number;
}

/** Clears the module cache and in-flight promise (tests). */
export function resetAvailableProgramsForTest(): void {
  inFlight?.controller.abort();
  cached = null;
  inFlight = null;
}

async function fetchServerInfoPrograms(signal: AbortSignal): Promise<ProgramOption[]> {
  const data = (await (await fetch("/api/server-info", { signal })).json()) as { programs?: string[] };
  if (!Array.isArray(data.programs)) return PROGRAMS;
  const staticValues = new Set(PROGRAMS.map((p) => p.value));
  const extra: ProgramOption[] = [];
  for (const fullPath of data.programs) {
    const basename = fullPath.split("/").pop() ?? fullPath;
    if (!staticValues.has(fullPath) && !staticValues.has(basename)) {
      extra.push({ value: basename, label: basename, description: fullPath });
    }
  }
  return extra.length > 0 ? [...PROGRAMS, ...extra] : PROGRAMS;
}

async function fetchPrograms(signal: AbortSignal): Promise<{ programs: ProgramOption[]; viaFallback: boolean }> {
  try {
    const client = createClient(SessionService, getConnectTransport());
    // Shared across consumers, so a per-component abort would cancel others' read.
    // abort-signal-exempt
    const res = await client.listProgramsConfig({});
    if (res.programs && res.programs.length > 0) {
      const programs = res.programs.map((p) => ({
        value: p.id,
        label: p.label,
        description: p.description || p.command,
        command: p.command,
        cliFlags: p.cliFlags,
      }));
      return { programs, viaFallback: false };
    }
    return { programs: PROGRAMS, viaFallback: false };
  } catch {
    return { programs: await fetchServerInfoPrograms(signal), viaFallback: true };
  }
}

/** Both the RPC and its /api/server-info fallback run inside the single in-flight promise; failures are not cached. */
function subscribe(): { promise: Promise<ProgramOption[]>; release: () => void } {
  if (cached && Date.now() - cached.at < CACHE_TTL_MS) {
    return { promise: Promise.resolve(cached.programs), release: () => {} };
  }
  if (!inFlight) {
    const controller = new AbortController();
    const flight: Flight = { controller, subscribers: 0, promise: undefined as unknown as Promise<ProgramOption[]> };
    flight.promise = fetchPrograms(controller.signal)
      .then(({ programs, viaFallback }) => {
        // The RPC failed to get here: share this result with concurrent mounts but let the next mount retry the RPC.
        if (!viaFallback) cached = { at: Date.now(), programs };
        return programs;
      })
      .finally(() => {
        if (inFlight === flight) inFlight = null;
      });
    flight.promise.catch(() => {});
    inFlight = flight;
  }
  const flight = inFlight;
  flight.subscribers++;
  let released = false;
  const release = () => {
    if (released) return;
    released = true;
    if (--flight.subscribers > 0) return;
    // Drop it now so a remount (StrictMode) starts a fresh flight instead of joining an aborted one.
    if (inFlight === flight) inFlight = null;
    flight.controller.abort();
  };
  return { promise: flight.promise, release };
}

export function useAvailablePrograms(): ProgramOption[] {
  const [programs, setPrograms] = useState<ProgramOption[]>(cached?.programs ?? PROGRAMS);

  useEffect(() => {
    // Guard against environments (e.g. tests) where fetch or connect transport is not configured
    if (typeof fetch !== "function") return;

    let mounted = true;
    const sub = subscribe();
    sub.promise
      .then((list) => {
        if (mounted) setPrograms(list);
      })
      .catch(() => {});

    return () => {
      mounted = false;
      sub.release();
    };
  }, []);

  return programs;
}
