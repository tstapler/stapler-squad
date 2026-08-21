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
