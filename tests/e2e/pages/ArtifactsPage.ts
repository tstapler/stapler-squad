import { expect, type Locator, type Page } from "@playwright/test";
import { baseUrl } from "../helpers/base-url";

const RPC = "**/api/session.v1.SessionService";

export interface MockRow {
  sessionId: string;
  sessionTitle: string;
  kind: "pr" | "commit" | "branch" | "plan_doc";
  /** PR number for kind=pr, otherwise the SHA, branch name or path. */
  value: string | number;
  state?: "open" | "merged" | "closed" | "unknown";
  source?: "session_pr" | "artifact";
  owner?: string;
  repo?: string;
  archived?: boolean;
  at?: string;
}

export interface MockHit {
  sessionId: string;
  title: string;
  source: "session_pr" | "artifact";
  matchType?: string;
  matchText?: string;
}

/** Connect-JSON form of one browse row. */
export function rowJson(r: MockRow): Record<string, unknown> {
  const owner = r.owner ?? "acme";
  const repo = r.repo ?? "app";
  const at = r.at ?? "2026-11-05T09:00:00Z";
  const body: Record<string, unknown> = {
    sessionId: r.sessionId,
    sessionTitle: r.sessionTitle,
    sessionArchived: r.archived ?? false,
    source: r.source ?? "artifact",
    record: { kind: r.kind, firstSeenAt: at },
  };
  const record = body.record as Record<string, unknown>;
  if (r.kind === "pr") {
    record.pr = { host: "github.com", owner, repo, number: r.value, source: "created" };
    body.pullRequest = { host: "github.com", owner, repo, number: r.value, state: r.state ?? "open", url: `https://github.com/${owner}/${repo}/pull/${r.value}`, pollError: "POLL_ERROR_NONE" };
  } else if (r.kind === "commit") {
    record.commit = { sha: String(r.value), subject: "fix: token refresh", branch: "feat-x" };
  } else if (r.kind === "branch") {
    record.branch = { name: String(r.value), action: "pushed" };
  } else {
    record.planDoc = { path: String(r.value) };
  }
  return body;
}

export function hitJson(h: MockHit): Record<string, unknown> {
  return {
    sessionId: h.sessionId,
    title: h.title,
    source: h.source,
    lastActivity: "2026-11-05T09:00:00Z",
    matches: [{ type: h.matchType ?? "pr", text: h.matchText ?? "acme/app#12", score: 1 }],
  };
}

/** The /artifacts page (design/ux.md S8, S9). Browse, search and PR lookup RPCs are answered by mocks: the isolated server has no scanned transcripts. */
export class ArtifactsPage {
  readonly root: Locator;
  readonly heading: Locator;
  readonly search: Locator;
  readonly searchSubmit: Locator;
  readonly clearSearch: Locator;
  readonly rows: Locator;
  readonly count: Locator;
  readonly empty: Locator;
  readonly error: Locator;
  readonly retry: Locator;
  readonly unscanned: Locator;
  readonly loadMore: Locator;
  readonly filterType: Locator;
  readonly filterRepo: Locator;
  readonly filterState: Locator;
  readonly filtersButton: Locator;
  readonly filterSheet: Locator;
  readonly requests: { browse: Record<string, unknown>[]; search: Record<string, unknown>[]; find: Record<string, unknown>[] } = { browse: [], search: [], find: [] };

  constructor(private readonly page: Page) {
    this.root = page.getByTestId("artifacts-page");
    this.heading = page.getByRole("heading", { name: "Artifacts", level: 1 });
    this.search = page.getByTestId("artifacts-search");
    this.searchSubmit = page.getByTestId("artifacts-search-submit");
    this.clearSearch = page.getByTestId("artifacts-clear-search");
    this.rows = page.getByTestId("artifact-row");
    this.count = page.getByTestId("artifacts-count");
    this.empty = page.getByTestId("artifacts-empty");
    this.error = page.getByTestId("artifacts-error");
    this.retry = page.getByTestId("artifacts-retry");
    this.unscanned = page.getByTestId("artifacts-unscanned");
    this.loadMore = page.getByTestId("artifacts-load-more");
    this.filterType = page.getByTestId("artifacts-filter-type");
    this.filterRepo = page.getByTestId("artifacts-filter-repo");
    this.filterState = page.getByTestId("artifacts-filter-state");
    this.filtersButton = page.getByTestId("artifacts-filters-button");
    this.filterSheet = page.getByTestId("artifacts-filter-sheet");
  }

  /** Answers ListSessionArtifacts with `respond(request)`; records each request body. */
  async mockBrowse(respond: (req: Record<string, unknown>) => Record<string, unknown> | null): Promise<void> {
    await this.page.route(`${RPC}/ListSessionArtifacts`, (route) => this.answer(route, this.requests.browse, respond));
  }

  async mockSearch(respond: (req: Record<string, unknown>) => Record<string, unknown> | null): Promise<void> {
    await this.page.route(`${RPC}/SearchSessionArtifacts`, (route) => this.answer(route, this.requests.search, respond));
  }

  async mockFind(respond: (req: Record<string, unknown>) => Record<string, unknown> | null): Promise<void> {
    await this.page.route(`${RPC}/FindSessionsByPullRequest`, (route) => this.answer(route, this.requests.find, respond));
  }

  /** Never answers: the request hangs until the page's own timeout aborts it. */
  async hangBrowse(): Promise<void> {
    await this.page.route(`${RPC}/ListSessionArtifacts`, () => new Promise<void>(() => undefined));
  }

  private async answer(
    route: import("@playwright/test").Route,
    log: Record<string, unknown>[],
    respond: (req: Record<string, unknown>) => Record<string, unknown> | null,
  ): Promise<void> {
    const req = (route.request().postDataJSON() ?? {}) as Record<string, unknown>;
    log.push(req);
    const body = respond(req);
    if (body === null) {
      await route.fulfill({ status: 500, contentType: "application/json", json: { code: "internal", message: "mocked failure" } });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", json: body });
  }

  async goto(query = ""): Promise<void> {
    await this.page.goto(`${baseUrl()}/artifacts${query}`, { waitUntil: "domcontentloaded", timeout: 15000 });
    await expect(this.heading).toBeVisible();
  }

  async submitSearch(text: string): Promise<void> {
    await this.search.fill(text);
    await this.searchSubmit.click();
  }

  sessionLink(row: Locator): Locator {
    return row.getByTestId("artifact-session-link");
  }
}
