import { APIRequestContext } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

/**
 * Seeds a backlog item directly in "queued" status, bypassing the real
 * WIP-cap spawn flow entirely.
 *
 * Backed by the `/api/debug/backlog/seed-queued` handler
 * (server/services/backlog_debug_seed_handler.go), registered ONLY when
 * STAPLER_SQUAD_INSTANCE=e2e-local (server.go) — never reachable in a normal
 * deploy. Mirrors seedStuckItem's approach (see ./StuckItemsPage.ts) for the
 * same reason: driving enough real session spawns to actually fill the
 * concurrency cap in an e2e test would be slow and flaky, so the queued
 * *display* is tested directly against seeded data; the dequeue *mechanics*
 * (FIFO order, CAS claim exclusivity, rollback on spawn failure) are covered
 * by Go unit/integration tests in server/services and session.
 */
export async function seedQueuedItem(request: APIRequestContext, title: string): Promise<string> {
  const resp = await request.post(`${BASE_URL}/api/debug/backlog/seed-queued`, {
    headers: { "Content-Type": "application/json" },
    data: { title },
  });
  if (!resp.ok()) {
    throw new Error(`seedQueuedItem failed (${resp.status()}): ${await resp.text().catch(() => "")}`);
  }
  const body = (await resp.json()) as { itemId: string };
  return body.itemId;
}
