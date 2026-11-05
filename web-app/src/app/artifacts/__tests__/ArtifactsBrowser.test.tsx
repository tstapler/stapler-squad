import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { timestampFromMs } from "@bufbuild/protobuf/wkt";
import { SessionArtifactHitSchema, SessionArtifactRowSchema, ArtifactMatchSchema } from "@/gen/session/v1/session_pb";
import { ArtifactPRRecordSchema, ArtifactRecordProtoSchema, ArtifactCommitRecordSchema, SessionPullRequestSchema } from "@/gen/session/v1/types_pb";
import { renderWithNav } from "@/lib/nav/testing";
import { usePathname, useSearchParams } from "@/lib/nav";
import { artifactsApi } from "@/lib/artifacts/artifactsApi";
import { ArtifactsBrowser } from "@/components/artifacts/ArtifactsBrowser";

jest.mock("@/lib/nav", () => jest.requireActual("@/lib/nav/impl.memory"));
const addNotification = jest.fn();
jest.mock("@/lib/contexts/NotificationContext", () => ({ useNotificationCommands: () => ({ addNotification }) }));
let compact = false;
jest.mock("@/lib/hooks/useMediaQuery", () => ({ useMediaQuery: () => compact }));
jest.mock("@/lib/artifacts/artifactsApi", () => ({
  PAGE_SIZE: 50,
  artifactsApi: { browse: jest.fn(), search: jest.fn(), findByPr: jest.fn() },
}));

const api = artifactsApi as jest.Mocked<typeof artifactsApi>;
const NOW = Date.now();

const prRow = create(SessionArtifactRowSchema, {
  sessionId: "u1",
  sessionTitle: "refactor-auth",
  source: "session_pr",
  pullRequest: create(SessionPullRequestSchema, { owner: "acme", repo: "app", number: 12, state: "open", url: "https://github.com/acme/app/pull/12" }),
  record: create(ArtifactRecordProtoSchema, {
    kind: "pr",
    firstSeenAt: timestampFromMs(NOW - 3_600_000),
    record: { case: "pr", value: create(ArtifactPRRecordSchema, { owner: "acme", repo: "app", number: 12 }) },
  }),
});
const commitRow = create(SessionArtifactRowSchema, {
  sessionId: "u1",
  sessionTitle: "refactor-auth",
  source: "artifact",
  record: create(ArtifactRecordProtoSchema, {
    kind: "commit",
    firstSeenAt: timestampFromMs(NOW - 7_200_000),
    record: { case: "commit", value: create(ArtifactCommitRecordSchema, { sha: "1a2b3c4d5e6f1a2b3c4d5e6f1a2b3c4d5e6f1a2b", subject: "fix: token refresh" }) },
  }),
});
const hit = (id: string, source: string) =>
  create(SessionArtifactHitSchema, { sessionId: id, title: `session-${id}`, source, matches: [create(ArtifactMatchSchema, { type: "pr", text: "acme/app#12" })] });

function Probe() {
  const sp = useSearchParams();
  return <div data-testid="loc">{usePathname()}?{sp.toString()}</div>;
}

function renderPage(entry: string) {
  return renderWithNav(<><Probe /><ArtifactsBrowser /></>, { initialEntries: [entry] });
}

beforeEach(() => {
  jest.clearAllMocks();
  compact = false;
  api.browse.mockResolvedValue({ rows: [prRow, commitRow], nextPageToken: "", totalCount: 2, unscanned: 0 });
});

describe("ArtifactsBrowser", () => {
  it("browses with the URL filters and shows a chip and a session link per PR record", async () => {
    renderPage("/artifacts?type=pr&repo=acme/app");
    await screen.findAllByTestId("artifact-row");
    expect(api.browse.mock.calls[0][0]).toMatchObject({ type: "pr", repo: "acme/app" });
    const row = screen.getAllByTestId("artifact-row")[0];
    expect(within(row).getByTestId("pr-chip")).toBeInTheDocument();
    expect(within(row).getByTestId("artifact-session-link")).toHaveAttribute("href", "/?session=u1&tab=artifacts");
    expect(within(row).getByText("Tracked")).toBeInTheDocument();
    expect(screen.getByTestId("artifacts-count")).toHaveTextContent("Showing 2 of 2");
  });

  it("renders a table on desktop and cards on mobile", async () => {
    const view = renderPage("/artifacts");
    await screen.findByTestId("artifacts-table");
    expect(screen.getAllByRole("columnheader").map((h) => h.getAttribute("scope"))).toEqual(["col", "col", "col", "col"]);
    view.unmount();
    compact = true;
    renderPage("/artifacts");
    await screen.findByTestId("artifacts-cards");
    expect(screen.getByTestId("artifacts-filters-button")).toBeInTheDocument();
    expect(screen.queryByTestId("artifacts-filter-bar")).toBeNull();
  });

  it("writes a changed filter to the URL and reloads", async () => {
    renderPage("/artifacts");
    await screen.findAllByTestId("artifact-row");
    fireEvent.change(screen.getByTestId("artifacts-filter-type"), { target: { value: "commit" } });
    await waitFor(() => expect(screen.getByTestId("loc")).toHaveTextContent("type=commit"));
    await waitFor(() => expect(api.browse).toHaveBeenCalledTimes(2));
    expect(api.browse.mock.calls[1][0].type).toBe("commit");
  });

  it("opens the filter sheet behind the Filters button and closes it with Done", async () => {
    compact = true;
    renderPage("/artifacts?type=pr");
    await screen.findByTestId("artifacts-cards");
    const button = screen.getByTestId("artifacts-filters-button");
    expect(button).toHaveTextContent("Filters (1)");
    fireEvent.click(button);
    expect(screen.getByRole("dialog", { name: "Filters" })).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("artifacts-filter-done"));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(button).toHaveFocus();
  });

  it("shows the unscanned notice", async () => {
    api.browse.mockResolvedValue({ rows: [prRow], nextPageToken: "", totalCount: 1, unscanned: 3 });
    renderPage("/artifacts");
    expect(await screen.findByTestId("artifacts-unscanned")).toHaveTextContent("3 sessions aren't scanned yet, so results may be incomplete");
  });

  it("shows distinct empty states for no artifacts and for filters that match nothing", async () => {
    api.browse.mockResolvedValue({ rows: [], nextPageToken: "", totalCount: 0, unscanned: 0 });
    const view = renderPage("/artifacts");
    expect(await screen.findByText(/No artifacts yet\./)).toBeInTheDocument();
    view.unmount();
    renderPage("/artifacts?type=pr");
    expect(await screen.findByText("No artifacts match these filters.")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Clear filters" }).length).toBeGreaterThan(0);
  });

  it("shows a browse error with Retry that reloads", async () => {
    api.browse.mockRejectedValueOnce(new Error("boom"));
    renderPage("/artifacts");
    expect(await screen.findByRole("alert")).toHaveTextContent("Couldn't load artifacts");
    fireEvent.click(screen.getByTestId("artifacts-retry"));
    await screen.findAllByTestId("artifact-row");
    expect(api.browse).toHaveBeenCalledTimes(2);
  });

  it("appends the next page on Load more and keeps rows when it fails", async () => {
    api.browse.mockResolvedValueOnce({ rows: [prRow], nextPageToken: "t1", totalCount: 2, unscanned: 0 });
    renderPage("/artifacts");
    await screen.findAllByTestId("artifact-row");
    api.browse.mockRejectedValueOnce(new Error("x"));
    fireEvent.click(screen.getByTestId("artifacts-load-more"));
    expect(await screen.findByTestId("artifacts-load-more-error")).toBeInTheDocument();
    expect(screen.getAllByTestId("artifact-row")).toHaveLength(1);
    api.browse.mockResolvedValueOnce({ rows: [commitRow], nextPageToken: "", totalCount: 2, unscanned: 0 });
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(screen.getAllByTestId("artifact-row")).toHaveLength(2));
    expect(api.browse.mock.calls[2][1]).toBe("t1");
  });
});

describe("ArtifactsBrowser PR lookup (S9)", () => {
  it("dispatches FindSessionsByPullRequest for ?pr= and lists several matches Tracked first", async () => {
    api.findByPr.mockResolvedValue({ hits: [hit("a", "session_pr"), hit("b", "artifact")], unscanned: 0 });
    renderPage("/artifacts?pr=acme/app%2312");
    await screen.findAllByTestId("artifact-row");
    expect(api.findByPr.mock.calls[0][0]).toEqual({ host: "", owner: "acme", repo: "app", number: 12 });
    expect(api.browse).not.toHaveBeenCalled();
    expect(screen.getAllByTestId("artifact-row").map((r) => r.textContent)).toEqual([
      expect.stringContaining("Tracked"),
      expect.stringContaining("Seen in transcript"),
    ]);
    expect(addNotification).not.toHaveBeenCalled();
  });

  it("opens the only matching session with a View all toast, once", async () => {
    api.findByPr.mockResolvedValue({ hits: [hit("a", "session_pr")], unscanned: 0 });
    renderPage("/artifacts?pr=acme/app%2312");
    await waitFor(() => expect(screen.getByTestId("loc")).toHaveTextContent("session=a"));
    expect(addNotification).toHaveBeenCalledTimes(1);
    const toast = addNotification.mock.calls[0][0];
    expect(toast).toMatchObject({ title: "Opened the session tracking acme/app#12 (Tracked)", message: "1 match.", viewLabel: "View all" });
    act(() => toast.onView());
    await waitFor(() => expect(screen.getByTestId("loc")).toHaveTextContent("view=all"));
  });

  it("keeps the list when view=all is set", async () => {
    api.findByPr.mockResolvedValue({ hits: [hit("a", "session_pr")], unscanned: 0 });
    renderPage("/artifacts?pr=acme/app%2312&view=all");
    await screen.findAllByTestId("artifact-row");
    expect(screen.getByTestId("loc")).toHaveTextContent("/artifacts");
    expect(addNotification).not.toHaveBeenCalled();
  });

  it("reports an unparseable pr filter without calling the server", async () => {
    renderPage("/artifacts?pr=nonsense");
    expect(await screen.findByTestId("artifacts-pr-invalid")).toBeInTheDocument();
    expect(api.findByPr).not.toHaveBeenCalled();
    expect(api.browse).not.toHaveBeenCalled();
  });

  it("shows the empty-search copy when nothing is tied to the PR", async () => {
    api.findByPr.mockResolvedValue({ hits: [], unscanned: 0 });
    renderPage("/artifacts?pr=acme/app%2312");
    expect(await screen.findByText(/No sessions found for "acme\/app#12"/)).toBeInTheDocument();
  });
});
