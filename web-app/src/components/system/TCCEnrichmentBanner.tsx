"use client";
// +feature: ui:tcc-enrichment-consent

import { useEffect, useMemo, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService, type TCCEnrichmentRoot } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { SystemBanner } from "@/components/ui/SystemBanner";

function useTCCEnrichment() {
  const client = useMemo(() => createClient(SessionService, getConnectTransport()), []);
  const [roots, setRoots] = useState<TCCEnrichmentRoot[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let mounted = true;
    const refresh = async () => {
      try {
        const result = await client.getTCCEnrichmentStatus({});
        if (mounted) setRoots(result.enabled ? result.roots : []);
      } catch {
        // Retain the last explanation on transient transport failures.
      }
    };
    void refresh();
    const timer = setInterval(refresh, 10_000);
    return () => { mounted = false; clearInterval(timer); };
  }, [client]);

  const proceed = async (id: string) => {
    setBusy(id);
    setError(null);
    try {
      await client.continueTCCEnrichment({ rootId: id });
      const result = await client.getTCCEnrichmentStatus({});
      setRoots(result.enabled ? result.roots : []);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(null);
    }
  };

  return { roots, busy, error, proceed };
}

export function TCCEnrichmentBanner() {
  const { roots, busy, error, proceed } = useTCCEnrichment();
  const [deferred, setDeferred] = useState<Set<string>>(new Set());
  return <>{roots.filter((root) => !deferred.has(root.id) && !(root.appConsented && root.outcome === "completed")).map((root) => (
    <SystemBanner
      key={root.id}
      id="tcc-enrichment-consent"
      severity="info"
      testId="tcc-enrichment-banner"
      message={<>
        Background worktree enrichment needs to read git metadata to identify repositories and worktrees in {root.root}.
        Affected sessions: {root.sessionTitles.length ? root.sessionTitles.join(", ") : root.paths.join(", ")}.
        {!root.appConsented ? <> This location may be protected by macOS. Continue when you are ready for a possible macOS permission request; macOS may not show one. Not now leaves this enrichment paused.</> : <> App consent saved. Enrichment: {root.outcome}. A slow or failed operation does not establish whether macOS permission is granted or denied; filesystem problems can also cause it.</>}
        {" "}This gates enrichment only, not other app or child-process filesystem access. Classification is heuristic, and app consent is not an OS grant.
        {error && <> Could not continue: {error}</>}
      </>}
      actions={[
        ...(!root.appConsented ? [{ label: "Continue to macOS permission request", onClick: () => { void proceed(root.id); }, disabled: busy !== null }] : []),
        { label: "Not now", onClick: () => setDeferred((current) => new Set(current).add(root.id)), disabled: busy === root.id },
      ]}
    />
  ))}</>;
}
