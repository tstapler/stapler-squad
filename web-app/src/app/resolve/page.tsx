"use client";
// +feature: backlog:deep-link-resolve

/**
 * Story 5.1 Task 2 (project_plans/backlog-deep-linking): the page an
 * ssq://<hostname>/<type>/<version>/<id> deep link lands on when it can't be
 * resolved directly by the OS scheme handler (e.g. a cross-host handoff
 * link, or an in-app "open this link" affordance) — see the Domain
 * Glossary's "in-app equivalent" URL. It reads the raw link out of the `url`
 * query param, calls the Surface 11 resolver endpoint
 * (GET /api/deep-link/resolve), and either:
 *   - navigates on to /backlog?item=<id> for a "local" resolution (matching
 *     the pre-existing legacy ?item=<uuid> flow exactly — Story 5.2),
 *   - navigates to the remote host's advertised address for a "handoff", or
 *   - renders DeepLinkErrorBanner for any of the "not-found"/"unreachable"/
 *     "invalid" failure kinds.
 *
 * Note this page is a different entry point from --open-url's local flow,
 * which resolves same-host links directly without ever calling this HTTP
 * endpoint or rendering this page.
 */

import { useCallback, useEffect, useState, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { usePageView } from "@/lib/analytics/usePageView";
import {
  DeepLinkErrorBanner,
  type DeepLinkFailureReason,
} from "@/components/DeepLinkErrorBanner";
import * as styles from "./resolve.css";

type ResolveResponse =
  | { kind: "local"; item: { ID?: string; PublicIDRaw?: string } }
  | { kind: "handoff"; advertisedAddress: string }
  | { kind: "not-found"; reason: "deleted" | "archived" }
  | { kind: "unreachable"; reason: "not-registered" | "unreachable"; lastSeenAt?: string }
  | { kind: "invalid"; reason: "malformed" | "version-mismatch" };

// extractHostname pulls the authority component out of a raw ssq:// URL for
// display purposes only (the DeepLinkErrorBanner's hostname prop) — the
// server is the sole source of truth for whether the link is valid, this is
// best-effort and never used to make a resolution decision client-side.
function extractHostname(raw: string): string | undefined {
  try {
    const u = new URL(raw);
    return u.host || undefined;
  } catch {
    return undefined;
  }
}

function ResolveDeepLinkInner() {
  usePageView();
  const router = useRouter();
  const searchParams = useSearchParams();
  const rawUrl = searchParams.get("url") ?? "";

  const [state, setState] = useState<
    | { status: "loading" }
    | { status: "error"; reason: DeepLinkFailureReason; lastSeenAt?: string; advertisedAddress?: string }
    | { status: "invalid-request" }
  >({ status: "loading" });

  const hostname = extractHostname(rawUrl);

  const resolve = useCallback(async () => {
    if (!rawUrl) {
      setState({ status: "invalid-request" });
      return;
    }
    setState({ status: "loading" });
    try {
      const res = await fetch(`/api/deep-link/resolve?url=${encodeURIComponent(rawUrl)}`);
      const data: ResolveResponse = await res.json();

      if (data.kind === "local") {
        const id = data.item?.PublicIDRaw || data.item?.ID;
        router.replace(id ? `/backlog?item=${encodeURIComponent(id)}` : "/backlog");
        return;
      }
      if (data.kind === "handoff") {
        window.location.href = data.advertisedAddress;
        return;
      }
      if (data.kind === "not-found") {
        setState({ status: "error", reason: data.reason });
        return;
      }
      if (data.kind === "unreachable") {
        setState({
          status: "error",
          reason: data.reason,
          lastSeenAt: data.lastSeenAt,
        });
        return;
      }
      // data.kind === "invalid"
      setState({ status: "error", reason: data.reason });
    } catch {
      // A network-level failure (fetch itself rejected) is treated the same
      // as "unreachable" — the item's own host, not this one, is at fault.
      setState({ status: "error", reason: "unreachable", advertisedAddress: undefined });
    }
  }, [rawUrl, router]);

  useEffect(() => {
    resolve();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rawUrl]);

  const goToBoard = useCallback(() => router.replace("/backlog/board"), [router]);
  const copyHostAddress = useCallback((address: string) => {
    if (typeof navigator !== "undefined" && navigator.clipboard) {
      navigator.clipboard.writeText(address);
    }
  }, []);

  if (state.status === "loading") {
    return (
      <div className={styles.container} data-testid="deep-link-resolve-loading">
        Resolving link…
      </div>
    );
  }

  if (state.status === "invalid-request") {
    return (
      <div className={styles.container}>
        <DeepLinkErrorBanner reason="malformed" onGoToBoard={goToBoard} />
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <DeepLinkErrorBanner
        reason={state.reason}
        hostname={hostname}
        lastSeenAt={state.lastSeenAt}
        advertisedAddress={state.advertisedAddress}
        onGoToBoard={goToBoard}
        onRetry={resolve}
        onCopyHostAddress={
          state.advertisedAddress
            ? () => copyHostAddress(state.advertisedAddress as string)
            : undefined
        }
      />
    </div>
  );
}

export default function ResolveDeepLinkPage() {
  return (
    <Suspense>
      <ResolveDeepLinkInner />
    </Suspense>
  );
}
