import { Page } from "@playwright/test";

/**
 * Mocks GET /api/deep-link/resolve (server/services/deep_link_resolver.go)
 * for deterministic e2e coverage of Surfaces 2-9
 * (project_plans/backlog-deep-linking) without needing a real second host
 * or Workspace Host Registry entry — response shapes mirror
 * web-app/src/app/resolve/page.tsx's `ResolveResponse` type exactly.
 */
export type MockResolveResponse =
  | { kind: "local"; item: { ID?: string; PublicIDRaw?: string } }
  | { kind: "handoff"; advertisedAddress: string }
  | { kind: "not-found"; reason: "deleted" | "archived" }
  | { kind: "unreachable"; reason: "not-registered" | "unreachable"; lastSeenAt?: string }
  | { kind: "invalid"; reason: "malformed" | "version-mismatch" };

const statusFor = (r: MockResolveResponse): number => {
  switch (r.kind) {
    case "local":
    case "handoff":
      return 200;
    case "not-found":
      return 404;
    case "unreachable":
      return 409;
    case "invalid":
      return 400;
  }
};

/**
 * Installs a route handler for /api/deep-link/resolve*. `responses` is
 * consumed in order, one per request, so a test can assert Retry behavior
 * (e.g. still-failing on every attempt, or succeeding on the Nth). The
 * last response repeats once the list is exhausted.
 */
export async function mockDeepLinkResolve(page: Page, responses: MockResolveResponse[]) {
  let callIndex = 0;
  await page.route("**/api/deep-link/resolve**", async (route) => {
    const response = responses[Math.min(callIndex, responses.length - 1)];
    callIndex += 1;
    await route.fulfill({
      status: statusFor(response),
      contentType: "application/json",
      body: JSON.stringify(response),
    });
  });
}

/** Builds the in-app `/resolve?url=...` URL for a raw ssq:// link. */
export function resolvePageUrl(rawSsqUrl: string): string {
  return `/resolve?url=${encodeURIComponent(rawSsqUrl)}`;
}
