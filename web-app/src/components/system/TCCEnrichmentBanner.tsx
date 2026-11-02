"use client";
// +feature: ui:tcc-enrichment-consent

import { useEffect, useMemo, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService, type TCCEnrichmentRoot } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { SystemBanner } from "@/components/ui/SystemBanner";

function useTCCEnrichment() {
  const client = useMemo(() => createClient(SessionService, getConnectTransport()), []);
  const acknowledged = useRef(new Set<string>());
 const mergeRoots = (incoming: TCCEnrichmentRoot[]) => incoming.map((root) => acknowledged.current.has(root.id) ? { ...root, appConsented: true } : root);
  const [roots, setRoots] = useState<TCCEnrichmentRoot[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [errors, setErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    let mounted = true;
    const refresh = async () => {
      try {
        const result = await client.getTCCEnrichmentStatus({});
        if (mounted) setRoots(result.enabled ? mergeRoots(result.roots) : []);
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
    setErrors((current) => ({ ...current, [id]: "" }));
    try {
      await client.continueTCCEnrichment({ rootId: id });
    } catch (e) {
      setErrors((current) => ({ ...current, [id]: `Could not save consent: ${String(e)}` }));
      setBusy(null);
      return;
    }
    acknowledged.current.add(id);
    setRoots((current) => current.map((root) => root.id === id ? { ...root, appConsented: true } : root));
    try {
      const result = await client.getTCCEnrichmentStatus({});
      setRoots(result.enabled ? mergeRoots(result.roots) : []);
    } catch (e) {
      setErrors((current) => ({ ...current, [id]: `Consent saved; enrichment may already be running. Status refresh failed: ${String(e)}` }));
    } finally {
      setBusy(null);
    }
  };

  return { roots, busy, errors, proceed };
}

export function TCCEnrichmentBanner() {
  const { roots, busy, errors, proceed } = useTCCEnrichment();
  const [deferred, setDeferred] = useState<Set<string>>(new Set());
  return <>{roots.filter((root) => !deferred.has(root.id) && !(root.appConsented && root.outcome === "completed")).map((root) => (
    <SystemBanner
      key={root.id}
      id="tcc-enrichment-consent"
      severity="info"
      testId="tcc-enrichment-banner"
      alwaysExpanded
      message={<>
        Background worktree enrichment needs to read git metadata to identify repositories and worktrees in {root.root}.
        Affected sessions: {root.sessionTitles.length ? root.sessionTitles.join(", ") : root.paths.join(", ")}.
        {!root.appConsented ? <> This location is protected by a known lexical policy or needs readiness approval because its filesystem type is unknown. No filesystem classification runs before approval. Conventional code/worktree locations are treated as unprotected heuristically; mount protection there cannot be guaranteed without I/O. Continue when you are ready for a possible macOS permission request; macOS may not show one. Not now leaves this enrichment paused.</> : <> App consent saved. Enrichment: {root.outcome}. A slow or failed operation does not establish whether macOS permission is granted or denied; filesystem problems can also cause it.</>}
        {root.outcome === "denied" && <> macOS may be blocking access to this location (the read failed with &quot;operation not permitted&quot;; immutable flags or device management can cause that too). If you denied the prompt, allow stapler-squad in System Settings{root.settingsUrl ? <> (<a href={root.settingsUrl} rel="noreferrer noopener" data-testid="tcc-settings-link">Open Privacy settings</a>)</> : null}, then choose Check again.</>}
        {" "}This gates enrichment only, not other app or child-process filesystem access. Classification is heuristic, and app consent is not an OS grant.
        {errors[root.id] && <span role="alert"> {errors[root.id]}</span>}
      </>}
      actions={[
        ...(root.appConsented && root.outcome === "denied" ? [{ label: "Check again", onClick: () => { void proceed(root.id); }, disabled: busy !== null }] : []),
        ...(!root.appConsented ? [{ label: "Continue to macOS permission request", onClick: () => { void proceed(root.id); }, disabled: busy !== null }] : []),
        { label: "Not now", onClick: () => setDeferred((current) => new Set(current).add(root.id)), disabled: busy === root.id },
      ]}
    />
  ))}</>;
}
