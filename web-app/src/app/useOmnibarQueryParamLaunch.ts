import { useEffect } from "react";
import type { useRouter, useSearchParams } from "next/navigation";
import type { Session } from "@/gen/session/v1/types_pb";
import type { AnalyticsProvider } from "@/lib/analytics/types";

/**
 * Detects `?new=true`, `?pr=<url>`, `?duplicate=<id>`, or
 * `?worktree=<path>&branch=<branch>` query params and opens the Omnibar
 * pre-filled accordingly, clearing the param from the URL. Extracted from
 * page.tsx's HomeContent — same behavior, isolated as its own concern.
 */
export function useOmnibarQueryParamLaunch(
  searchParams: ReturnType<typeof useSearchParams>,
  router: ReturnType<typeof useRouter>,
  openOmnibar: (initialInput?: string, initialTitle?: string) => void,
  track: AnalyticsProvider["track"],
  getSession: (id: string) => Promise<Session | null>
) {
  useEffect(() => {
    const newParam = searchParams.get("new");
    const prUrl = searchParams.get("pr");
    const duplicateId = searchParams.get("duplicate");
    const worktreePath = searchParams.get("worktree");
    const worktreeBranch = searchParams.get("branch");
    const title = searchParams.get("title");

    if (prUrl) {
      router.replace("/", { scroll: false });
      openOmnibar(prUrl);
    } else if (newParam === "true") {
      router.replace("/", { scroll: false });
      openOmnibar();
    } else if (duplicateId) {
      router.replace("/", { scroll: false });
      track({ name: "session_duplicate_initiated", category: "user_action" });
      getSession(duplicateId)
        .then((session) => {
          openOmnibar(session?.repoRoot);
        })
        .catch(() => {
          openOmnibar();
        });
    } else if (worktreePath) {
      router.replace("/", { scroll: false });
      // Pass path@branch so the PathWithBranch detector pre-fills both fields
      openOmnibar(worktreeBranch ? `${worktreePath}@${worktreeBranch}` : worktreePath, title || undefined);
    }
  }, [searchParams, getSession, openOmnibar, router, track]);
}
