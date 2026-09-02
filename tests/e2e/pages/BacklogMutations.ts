import { APIRequestContext } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

/**
 * Wrappers around the `/api/debug/backlog/mutate-*` endpoints
 * (server/services/backlog_debug_mutate_handler.go), added alongside this
 * file for project_plans/backlog-event-driven-updates's Playwright e2e
 * layer.
 *
 * These call storage.TransitionBacklogItemStatus/UpdateBacklogItem/
 * ArchiveBacklogItem/DeleteBacklogItem directly — the same "no RPC handler
 * involved" path validation.md's Happy Path Scenario describes a reconciler
 * using — so a test can simulate a second, independent actor (reconciler,
 * another operator, another browser tab) mutating an item without needing a
 * real second browser context, and without fighting
 * TransitionBacklogItemStatus's real CanTransition/ValidateGates business
 * rules (plan approval, AC criteria, review verdict, etc.) that a
 * legitimate multi-step UI flow would otherwise require.
 *
 * Only reachable when the test server is started with
 * STAPLER_SQUAD_INSTANCE=e2e-local (server.go) — never registered in a
 * normal deploy.
 */

export async function createBacklogItemDirect(
  request: APIRequestContext,
  opts: {
    title: string;
    description?: string;
    status?: string;
    priority?: number;
    repoPath?: string;
    acCriteria?: string[];
  }
): Promise<string> {
  const resp = await request.post(`${BASE_URL}/api/debug/backlog/mutate-create`, {
    headers: { "Content-Type": "application/json" },
    data: {
      title: opts.title,
      description: opts.description ?? "",
      status: opts.status ?? "idea",
      priority: opts.priority ?? 3,
      repoPath: opts.repoPath ?? "",
      acCriteria: opts.acCriteria ?? [],
    },
  });
  if (!resp.ok()) {
    throw new Error(`createBacklogItemDirect failed (${resp.status()}): ${await resp.text().catch(() => "")}`);
  }
  const body = (await resp.json()) as { itemId: string };
  return body.itemId;
}

/**
 * Like createBacklogItemDirect, but also returns the item's publicId
 * (BacklogItemID, e.g. "bl_01J...") minted at creation time (Story 1.1 AC,
 * session/ent_repository_backlog.go's CreateBacklogItem) -- needed by tests
 * that assert against the UI's displayed/copied ID, which prefers publicId
 * over the raw UUID (Story 2.3, web-app/src/components/backlog/BacklogItemDetail.tsx).
 */
export async function createBacklogItemDirectWithPublicId(
  request: APIRequestContext,
  opts: {
    title: string;
    description?: string;
    status?: string;
    priority?: number;
    repoPath?: string;
    acCriteria?: string[];
  }
): Promise<{ itemId: string; publicId: string }> {
  const resp = await request.post(`${BASE_URL}/api/debug/backlog/mutate-create`, {
    headers: { "Content-Type": "application/json" },
    data: {
      title: opts.title,
      description: opts.description ?? "",
      status: opts.status ?? "idea",
      priority: opts.priority ?? 3,
      repoPath: opts.repoPath ?? "",
      acCriteria: opts.acCriteria ?? [],
    },
  });
  if (!resp.ok()) {
    throw new Error(`createBacklogItemDirectWithPublicId failed (${resp.status()}): ${await resp.text().catch(() => "")}`);
  }
  const body = (await resp.json()) as { itemId: string; publicId: string };
  return { itemId: body.itemId, publicId: body.publicId };
}

export async function transitionBacklogItemDirect(
  request: APIRequestContext,
  itemId: string,
  targetStatus: string
): Promise<void> {
  const resp = await request.post(`${BASE_URL}/api/debug/backlog/mutate-transition`, {
    headers: { "Content-Type": "application/json" },
    data: { itemId, targetStatus },
  });
  if (!resp.ok()) {
    throw new Error(`transitionBacklogItemDirect failed (${resp.status()}): ${await resp.text().catch(() => "")}`);
  }
}

export async function updateBacklogItemDirect(
  request: APIRequestContext,
  itemId: string,
  fields: { title?: string; description?: string }
): Promise<void> {
  const resp = await request.post(`${BASE_URL}/api/debug/backlog/mutate-update`, {
    headers: { "Content-Type": "application/json" },
    data: { itemId, ...fields },
  });
  if (!resp.ok()) {
    throw new Error(`updateBacklogItemDirect failed (${resp.status()}): ${await resp.text().catch(() => "")}`);
  }
}

export async function archiveBacklogItemDirect(request: APIRequestContext, itemId: string): Promise<void> {
  const resp = await request.post(`${BASE_URL}/api/debug/backlog/mutate-archive`, {
    headers: { "Content-Type": "application/json" },
    data: { itemId },
  });
  if (!resp.ok()) {
    throw new Error(`archiveBacklogItemDirect failed (${resp.status()}): ${await resp.text().catch(() => "")}`);
  }
}

export async function deleteBacklogItemDirect(request: APIRequestContext, itemId: string): Promise<void> {
  const resp = await request.post(`${BASE_URL}/api/debug/backlog/mutate-delete`, {
    headers: { "Content-Type": "application/json" },
    data: { itemId },
  });
  if (!resp.ok()) {
    throw new Error(`deleteBacklogItemDirect failed (${resp.status()}): ${await resp.text().catch(() => "")}`);
  }
}

/**
 * Wraps `/api/debug/backlog/seed-work-item-session` (server/services/
 * backlog_debug_seed_handler.go's handleSeedWorkItemSession) — creates a
 * backlog item with a linked "work" ItemSession (no backing Session/Worktree
 * row) so a Playwright test can reach ReviewChangesModal's real "View
 * Changes" trigger (gated only on a truthy work session, not a worktreePath)
 * without a real git worktree/tmux session spin-up.
 */
export async function seedWorkItemSessionDirect(
  request: APIRequestContext,
  opts: { title: string; status?: string }
): Promise<{ itemId: string; sessionId: string }> {
  const resp = await request.post(`${BASE_URL}/api/debug/backlog/seed-work-item-session`, {
    headers: { "Content-Type": "application/json" },
    data: { title: opts.title, status: opts.status ?? "review" },
  });
  if (!resp.ok()) {
    throw new Error(`seedWorkItemSessionDirect failed (${resp.status()}): ${await resp.text().catch(() => "")}`);
  }
  return (await resp.json()) as { itemId: string; sessionId: string };
}

/**
 * Wraps `/api/debug/backlog/seed-work-session-with-worktree` (server/
 * services/backlog_debug_seed_handler.go's handleSeedWorkSessionWithWorktree)
 * — creates a backlog item with a real (DB-only, no live tmux) work session
 * backed by a real Session+Worktree row pointing at a real temp directory,
 * so a Playwright test can reach BacklogFileBrowserModal's real "Browse
 * files in this worktree" trigger, which is gated on a truthy worktreePath
 * (unlike ReviewChangesModal's trigger — see seedWorkItemSessionDirect).
 */
export async function seedWorkSessionWithWorktreeDirect(
  request: APIRequestContext,
  opts: { title: string; status?: string; repoPath?: string }
): Promise<{ itemId: string; sessionId: string; worktreePath: string }> {
  const resp = await request.post(`${BASE_URL}/api/debug/backlog/seed-work-session-with-worktree`, {
    headers: { "Content-Type": "application/json" },
    data: { title: opts.title, status: opts.status ?? "review", repoPath: opts.repoPath ?? "" },
  });
  if (!resp.ok()) {
    throw new Error(`seedWorkSessionWithWorktreeDirect failed (${resp.status()}): ${await resp.text().catch(() => "")}`);
  }
  return (await resp.json()) as { itemId: string; sessionId: string; worktreePath: string };
}

/**
 * Wraps `/api/debug/backlog/seed-jules-work-session` (server/services/
 * backlog_debug_seed_handler.go's handleSeedJulesWorkSession) — creates a
 * backlog item with a linked jules_work ItemSession (Story 3.3.2), directly
 * through storage, no real Jules API dispatch/poll involved.
 *
 * `ended`/`endReason` select which of computeJulesPhase's (SessionsSection.tsx)
 * two closed-row outcomes the badge resolves to: `endReason: "jules_completed"`
 * (or `"jules_completed_no_pr"`) -> "done"; any other end reason (e.g.
 * `"jules_failed"`) -> "failed". Leaving `ended` unset produces "running".
 * "queued" and "needs-review" are not reachable through this real data path
 * — computeJulesPhase only branches on open-vs-closed — and
 * "reconnect-required" needs live JulesSessionPoller auth-failure state this
 * debug endpoint does not simulate; see jules-status-badge.spec.ts's header
 * comment for the full scope note.
 *
 * `itemId`, when set, attaches the seeded jules_work ItemSession to that
 * already-existing item instead of creating a new one (title/status are then
 * ignored) — used by jules-dispatch.spec.ts's §7.2 scenario to make the row
 * BacklogItemDetail's real WatchBacklogItems live-update path renders appear
 * on the exact item a `DispatchToJules` call (intercepted client-side to
 * avoid a live billed Jules API round trip — no e2e-local credential can
 * pass jules/source_registry.go's real ListSources check) was aimed at.
 */
export async function seedJulesWorkSessionDirect(
  request: APIRequestContext,
  opts: {
    title: string;
    status?: string;
    ended?: boolean;
    endReason?: string;
    prNumber?: number;
    prUrl?: string;
    itemId?: string;
  }
): Promise<{ itemId: string; sessionId: string }> {
  const resp = await request.post(`${BASE_URL}/api/debug/backlog/seed-jules-work-session`, {
    headers: { "Content-Type": "application/json" },
    data: {
      title: opts.title,
      status: opts.status ?? "review",
      ended: opts.ended ?? false,
      endReason: opts.endReason ?? "",
      prNumber: opts.prNumber ?? 0,
      prUrl: opts.prUrl ?? "",
      itemId: opts.itemId ?? "",
    },
  });
  if (!resp.ok()) {
    throw new Error(`seedJulesWorkSessionDirect failed (${resp.status()}): ${await resp.text().catch(() => "")}`);
  }
  return (await resp.json()) as { itemId: string; sessionId: string };
}

export async function enableBacklogFeatureFlag(request: APIRequestContext): Promise<void> {
  await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
    headers: { "Content-Type": "application/json" },
    data: { name: "backlog", enabled: true },
  });
}

export async function disableBacklogFeatureFlag(request: APIRequestContext): Promise<void> {
  await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
    headers: { "Content-Type": "application/json" },
    data: { name: "backlog", enabled: false },
  });
}
