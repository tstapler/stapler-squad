"use client";

import { useEffect, useRef, useState } from "react";
import { useRemotesService } from "@/lib/hooks/useRemotesService";
import type { RemoteOption } from "@/components/sessions/OmnibarCreationPanel";

export interface UseConfiguredRemotesResult {
  remotes: RemoteOption[];
  loading: boolean;
}

// useConfiguredRemotes fetches the configured-remotes list (ListRemotes) once on mount and
// maps it to the minimal RemoteOption[] shape the Omnibar's "Remote host" selector needs
// (ADR-001: remote-as-orthogonal-flag). This is the wiring OmnibarCreationPanel.tsx's and
// Omnibar.tsx's own "TODO(Phase 6): ... sourced from a remotesSlice ... populated by a
// ListRemotes RPC" comments called for -- until this hook existed, OmnibarContext.tsx never
// passed a `remotes` prop to <Omnibar> at all, so the selector never rendered in the actual
// running app regardless of what was configured in Settings -> Remotes (ssh-remote-workspaces
// Phase 6 Epic 6.3, found while wiring up remote-workspaces.spec.ts's e2e coverage of "create a
// session against a remote via the Omnibar").
//
// Mirrors useLauncherPresets' fetch-once-on-mount pattern (a fresh client per hook instance,
// no polling -- the list only changes when the user visits Settings -> Remotes, which is rare
// enough that a stale list for the lifetime of one page session is an acceptable tradeoff,
// matching launcher presets' same tradeoff).
export function useConfiguredRemotes(): UseConfiguredRemotesResult {
  const { listRemotes } = useRemotesService();
  const listRemotesRef = useRef(listRemotes);
  listRemotesRef.current = listRemotes;

  const [remotes, setRemotes] = useState<RemoteOption[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    async function fetchRemotes() {
      try {
        const list = await listRemotesRef.current();
        if (!cancelled) {
          setRemotes(list.map((r) => ({ name: r.name })));
        }
      } catch {
        // Best-effort: a failed fetch just leaves the selector hidden (empty
        // list), matching today's no-remotes-configured behavior rather than
        // surfacing a fetch error in the session-creation UI.
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    void fetchRemotes();
    return () => {
      cancelled = true;
    };
  }, []);

  return { remotes, loading };
}
