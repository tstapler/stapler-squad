import React from "react";
import { render, screen, fireEvent, act, within, waitFor } from "@testing-library/react";
import { create, type MessageInitShape } from "@bufbuild/protobuf";
import { ArtifactsTab, SCAN_SLOW_AFTER_MS } from "../ArtifactsTab";
import { SessionArtifactsSchema } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";
import { ArtifactRecordProtoSchema } from "@/gen/session/v1/types_pb";
import { timestampFromMs } from "@bufbuild/protobuf/wkt";
import { makePr } from "@/lib/pr/__testUtils__/pullRequests";

const mockAnnounce = jest.fn();
jest.mock("@/lib/hooks/useAnnounce", () => ({
  ...jest.requireActual("@/lib/hooks/useAnnounce"),
  useAnnounce: () => ({ announce: mockAnnounce, announceArrival: jest.fn() }),
}));

type ArtifactsInit = MessageInitShape<typeof SessionArtifactsSchema>;

function makeSession(artifactsFields?: ArtifactsInit): Session {
  const artifacts = artifactsFields != null
    ? create(SessionArtifactsSchema, artifactsFields)
    : undefined;
  return { artifacts } as unknown as Session;
}

// Cast a plain artifact literal to the protobuf type without importing the full runtime.
function makeArtifacts(a: {
  prUrls?: string[];
  commitShas?: string[];
  externalUrls?: string[];
  scanState?: string;
  transcriptSupported?: boolean;
  lastScannedAt?: { seconds: bigint; nanos: number };
}): Session["artifacts"] {
  return a as unknown as Session["artifacts"];
}

const setClipboard = (value: unknown) => Object.defineProperty(navigator, "clipboard", { value, configurable: true, writable: true });

describe("ArtifactsTab", () => {
  it("ArtifactsTab_should_showScanning_When_artifactsIsNull", () => {
    render(<ArtifactsTab session={makeSession(undefined)} />);
    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");
    expect(screen.getByText(/Scanning transcript/)).toBeInTheDocument();
    expect(screen.queryByText(/Extraction pending/)).not.toBeInTheDocument();
  });

  it("ArtifactsTab_should_showNoArtifacts_When_artifactsIsEmptyArrays", () => {
    render(
      <ArtifactsTab
        session={makeSession(makeArtifacts({ prUrls: [], commitShas: [], externalUrls: [] }))}
      />
    );
    expect(screen.getByText(/No artifacts found/)).toBeInTheDocument();
  });

  it("ArtifactsTab_should_renderPRLinks_When_artifactsHasPRURLs", () => {
    render(
      <ArtifactsTab
        session={makeSession(makeArtifacts({
          prUrls: ["https://github.com/owner/repo/pull/42"],
          commitShas: [],
          externalUrls: [],
        }))}
      />
    );
    expect(screen.getByText("owner/repo#42")).toBeInTheDocument();
    const link = screen.getByRole("link", { name: "owner/repo#42" });
    expect(link).toHaveAttribute("href", "https://github.com/owner/repo/pull/42");
    expect(link).toHaveAttribute("target", "_blank");
  });

  it("ArtifactsTab_should_truncateURLs_When_URLExceeds60Chars", () => {
    const longURL = "https://example.com/" + "a".repeat(60);
    render(
      <ArtifactsTab
        session={makeSession(makeArtifacts({
          prUrls: [],
          commitShas: [],
          externalUrls: [longURL],
        }))}
      />
    );
    // External URLs are behind a disclosure toggle — click to expand.
    fireEvent.click(screen.getByRole("button", { name: /Show 1 external URL/ }));
    const link = screen.getByRole("link", { name: /…$/ });
    expect(link).toBeInTheDocument();
    expect(link.textContent?.length ?? 0).toBeLessThan(longURL.length);
    expect(link).toHaveAttribute("href", longURL);
  });

  it("ArtifactsTab_should_addSecurityAttrsToExternalLinks", () => {
    render(
      <ArtifactsTab
        session={makeSession(makeArtifacts({
          prUrls: ["https://github.com/owner/repo/pull/42"],
          commitShas: [],
          externalUrls: [],
        }))}
      />
    );
    const link = screen.getByRole("link", { name: "owner/repo#42" });
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("ArtifactsTab_should_renderMultiplePRLinks", () => {
    render(
      <ArtifactsTab
        session={makeSession(makeArtifacts({
          prUrls: [
            "https://github.com/owner/repo/pull/1",
            "https://github.com/owner/repo/pull/2",
          ],
          commitShas: [],
          externalUrls: [],
        }))}
      />
    );
    expect(screen.getByText("owner/repo#1")).toBeInTheDocument();
    expect(screen.getByText("owner/repo#2")).toBeInTheDocument();
  });

  it("ArtifactsTab_should_renderRawURLForMalformedPRURL", () => {
    // A URL without /pull/ should render as-is (parsePRDisplay returns the raw URL)
    render(
      <ArtifactsTab
        session={makeSession(makeArtifacts({
          prUrls: ["https://github.com/owner/repo/issues/5"],
          commitShas: [],
          externalUrls: [],
        }))}
      />
    );
    // parsePRDisplay returns the raw URL when it can't parse as a PR URL
    expect(screen.getByRole("link")).toBeInTheDocument();
  });

  it("ArtifactsTab_should_renderCommitSHAs_When_artifactsHasCommits", () => {
    const sha = "abc123def456abc123def456abc123def456abc1";
    render(
      <ArtifactsTab
        session={makeSession(makeArtifacts({
          prUrls: [],
          commitShas: [sha],
          externalUrls: [],
        }))}
      />
    );
    // Renders the shortened 7-char prefix in a <code> element.
    expect(screen.getByText("abc123d")).toBeInTheDocument();
  });

  describe("scan states", () => {
    it("ArtifactsTab_should_explainNoTranscript_When_notLinkedAndProgramUnsupported", () => {
      render(<ArtifactsTab session={makeSession(makeArtifacts({ scanState: "not_linked", transcriptSupported: false }))} onRescan={jest.fn()} />);
      expect(screen.getByText(/does not produce a transcript/)).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Rescan" })).not.toBeInTheDocument();
    });

    it("ArtifactsTab_should_explainWaitingForLink_When_notLinkedAndProgramSupported", () => {
      render(<ArtifactsTab session={makeSession(makeArtifacts({ scanState: "not_linked", transcriptSupported: true }))} onRescan={jest.fn()} />);
      expect(screen.getByText(/no linked transcript yet/)).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Rescan" })).not.toBeInTheDocument();
    });

    it("ArtifactsTab_should_showNoneFoundWithRescan_When_scanFoundNothing", () => {
      const onRescan = jest.fn();
      render(<ArtifactsTab session={makeSession(makeArtifacts({ scanState: "none_found", transcriptSupported: true }))} onRescan={onRescan} />);
      expect(screen.getByText(/No artifacts found/)).toBeInTheDocument();
      expect(screen.queryByText(/Extraction pending/)).not.toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: "Rescan" }));
      expect(onRescan).toHaveBeenCalledTimes(1);
    });

    it("ArtifactsTab_should_showRescan_When_hasData", () => {
      render(<ArtifactsTab session={makeSession(makeArtifacts({ scanState: "has_data", prUrls: ["https://github.com/o/r/pull/1"] }))} onRescan={jest.fn()} />);
      expect(screen.getByRole("button", { name: "Rescan" })).toBeInTheDocument();
      expect(screen.getByText("o/r#1")).toBeInTheDocument();
    });

    it("ArtifactsTab_should_showAlertAndKeepLastGoodData_When_scanFailed", () => {
      const onRescan = jest.fn();
      render(<ArtifactsTab session={makeSession(makeArtifacts({ scanState: "failed", prUrls: ["https://github.com/o/r/pull/9"] }))} onRescan={onRescan} />);
      const alert = screen.getByRole("alert");
      expect(alert).toHaveTextContent(/Scan failed/);
      expect(screen.getByText("o/r#9")).toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: "Retry" }));
      expect(onRescan).toHaveBeenCalledTimes(1);
      fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });

    describe("manual rescan (UX-S7-2)", () => {
    const at = (n: number) => ({ seconds: BigInt(n), nanos: 0 });
    const noneFound = (n: number) =>
      makeSession(makeArtifacts({ scanState: "none_found", lastScannedAt: at(n) }));

    beforeEach(() => mockAnnounce.mockClear());

    it("ArtifactsTab_should_disableRescanAndAnnounceResult_When_rescanCompletes", async () => {
      const onRescan = jest.fn().mockResolvedValue(undefined);
      const { rerender } = render(<ArtifactsTab session={noneFound(100)} onRescan={onRescan} />);

      fireEvent.click(screen.getByRole("button", { name: "Rescan" }));
      const busy = await screen.findByRole("button", { name: /Rescanning/ });
      expect(busy).toBeDisabled();
      expect(busy).toHaveAttribute("aria-busy", "true");
      fireEvent.click(busy); // double click is ignored
      expect(onRescan).toHaveBeenCalledTimes(1);

      rerender(
        <ArtifactsTab
          session={makeSession(makeArtifacts({
            scanState: "has_data",
            prUrls: ["https://github.com/o/r/pull/1"],
            lastScannedAt: at(200),
          }))}
          onRescan={onRescan}
        />
      );
      expect(await screen.findByRole("button", { name: "Rescan" })).toBeEnabled();
      expect(mockAnnounce).toHaveBeenCalledWith("Scan finished: 1 artifact", "polite", "artifacts-rescan");
    });

    it("ArtifactsTab_should_announceNoArtifacts_When_rescanFindsNothing", async () => {
      const { rerender } = render(<ArtifactsTab session={noneFound(100)} onRescan={jest.fn().mockResolvedValue(undefined)} />);
      fireEvent.click(screen.getByRole("button", { name: "Rescan" }));
      await screen.findByRole("button", { name: /Rescanning/ });
      rerender(<ArtifactsTab session={noneFound(300)} onRescan={jest.fn()} />);
      await screen.findByRole("button", { name: "Rescan" });
      expect(mockAnnounce).toHaveBeenCalledWith("No artifacts found", "polite", "artifacts-rescan");
    });

    it("ArtifactsTab_should_showAlertAndReenableRescan_When_rescanRpcFails", async () => {
      const onRescan = jest.fn().mockRejectedValue(new Error("boom"));
      render(<ArtifactsTab session={noneFound(100)} onRescan={onRescan} />);
      fireEvent.click(screen.getByRole("button", { name: "Rescan" }));
      const alert = await screen.findByRole("alert");
      expect(alert).toHaveTextContent("Rescan failed: boom");
      expect(screen.getAllByRole("button", { name: "Rescan" })[0]).toBeEnabled();
      expect(screen.getByRole("button", { name: "Retry" })).toBeEnabled();
    });
  });

  describe("slow scan escape (UX-S7-4)", () => {
      beforeEach(() => jest.useFakeTimers());
      afterEach(() => jest.useRealTimers());

      it("ArtifactsTab_should_offerRescanAndCancelWaiting_When_scanningOver60s", () => {
        const onRescan = jest.fn().mockResolvedValue(undefined);
        render(<ArtifactsTab session={makeSession(makeArtifacts({ scanState: "scanning" }))} onRescan={onRescan} />);
        expect(screen.queryByText(/Taking longer than usual/)).not.toBeInTheDocument();
        act(() => { jest.advanceTimersByTime(SCAN_SLOW_AFTER_MS - 1); });
        expect(screen.queryByText(/Taking longer than usual/)).not.toBeInTheDocument();
        act(() => { jest.advanceTimersByTime(1); });
        expect(screen.getByText(/Taking longer than usual/)).toBeInTheDocument();
        fireEvent.click(screen.getByRole("button", { name: "Rescan" }));
        expect(onRescan).toHaveBeenCalledTimes(1);
      });

      it("ArtifactsTab_should_returnToPreviousData_When_cancelWaitingClicked", () => {
        render(<ArtifactsTab session={makeSession(makeArtifacts({ scanState: "scanning", prUrls: ["https://github.com/o/r/pull/3"] }))} onRescan={jest.fn()} />);
        act(() => { jest.advanceTimersByTime(SCAN_SLOW_AFTER_MS); });
        fireEvent.click(screen.getByRole("button", { name: "Cancel waiting" }));
        expect(screen.getByText("o/r#3")).toBeInTheDocument();
        expect(screen.queryByText(/Taking longer than usual/)).not.toBeInTheDocument();
      });

      it("ArtifactsTab_should_resetTimer_When_stateLeavesScanning", () => {
        const { rerender } = render(<ArtifactsTab session={makeSession(makeArtifacts({ scanState: "scanning" }))} />);
        act(() => { jest.advanceTimersByTime(SCAN_SLOW_AFTER_MS - 1000); });
        rerender(<ArtifactsTab session={makeSession(makeArtifacts({ scanState: "none_found" }))} />);
        act(() => { jest.advanceTimersByTime(5000); });
        expect(screen.queryByText(/Taking longer than usual/)).not.toBeInTheDocument();
        expect(screen.getByText(/No artifacts found/)).toBeInTheDocument();
      });
    });
  });
});

describe("grouped view (ux.md S7)", () => {
  const at = (hh: number, mm: number) => Date.UTC(2026, 10, 5, hh, mm);
  const rec = (ms: number, record: object) => create(ArtifactRecordProtoSchema, { firstSeenAt: timestampFromMs(ms), record } as never);
  const prRec = (ms: number, number: number, source = "created") => rec(ms, { case: "pr", value: { owner: "acme", repo: "app", number, source } });
  const commitRec = (ms: number, sha: string, subject = "") => rec(ms, { case: "commit", value: { sha, subject, branch: "feat-b" } });
  const planRec = (ms: number, path: string) => rec(ms, { case: "planDoc", value: { path } });
  const grouped = (records: ReturnType<typeof rec>[], session: Partial<Session> = {}) =>
    ({ artifacts: create(SessionArtifactsSchema, { scanState: "has_data", records }), pullRequests: [], ...session }) as unknown as Session;

  it("ArtifactsTab should group commit under PR created before it and plan doc under later PR and rest under Unassigned", () => {
    render(<ArtifactsTab session={grouped([
      commitRec(at(9, 0), "aaaaaaa1111", "early"),
      prRec(at(10, 0), 11),
      commitRec(at(10, 30), "1a2b3c4d5e6f", "fix: token refresh"),
      prRec(at(11, 0), 12),
      planRec(at(11, 15), "project_plans/x/plan.md"),
    ])} />);
    expect(within(screen.getByTestId("artifact-group-11")).getByRole("button", { name: "Copy commit SHA 1a2b3c4" })).toBeInTheDocument();
    expect(within(screen.getByTestId("artifact-group-12")).getByRole("button", { name: "Copy plan path project_plans/x/plan.md" })).toBeInTheDocument();
    expect(within(screen.getByTestId("artifact-group-unassigned")).getByRole("button", { name: "Copy commit SHA aaaaaaa" })).toBeInTheDocument();
    expect(screen.getByText("Grouped by when each item appeared relative to PR creation.")).toBeInTheDocument();
  });

  it("ArtifactsTab should fall back to flat v1 list when records missing", () => {
    render(<ArtifactsTab session={makeSession(makeArtifacts({ prUrls: ["https://github.com/o/r/pull/1"], commitShas: ["a".repeat(40)], externalUrls: [], scanState: "has_data" }))} />);
    expect(screen.queryByTestId("artifacts-grouped")).toBeNull();
    expect(screen.getByRole("heading", { name: "Pull Requests" })).toBeInTheDocument();
  });

  it("mentioned PRs should be collapsed and labeled not created by this session", () => {
    render(<ArtifactsTab session={grouped([prRec(at(10, 0), 11), prRec(at(10, 5), 40, "mentioned")])} />);
    const group = screen.getByTestId("artifact-group-mentioned");
    const toggle = within(group).getByRole("button", { name: /Mentioned pull requests/ });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(within(group).queryByText(/not created by this session/)).toBeNull();
    fireEvent.click(toggle);
    expect(within(group).getByText("Mentioned, not created by this session.")).toBeInTheDocument();
    expect(within(group).getByRole("link", { name: "#40 acme/app" })).toBeInTheDocument();
  });

  it("tab should show redacted marker never the token value", () => {
    render(<ArtifactsTab session={grouped([prRec(at(10, 0), 11), rec(at(10, 1), { case: "ciLink", value: { url: "https://ci.example/run?token=[redacted]" } })])} />);
    expect(screen.getByTestId("artifact-redacted")).toHaveTextContent("[redacted]");
    expect(screen.queryByText(/ci\.example/)).toBeNull();
    expect(screen.queryByRole("link", { name: /ci\.example/ })).toBeNull();
  });

  it("group header should use the PR chip for a tracked PR and a plain link otherwise", () => {
    const session = grouped([prRec(at(10, 0), 11), prRec(at(11, 0), 12)], { pullRequests: [makePr({ number: 12, state: "merged" })] } as never);
    render(<ArtifactsTab session={session} />);
    expect(within(screen.getByTestId("artifact-group-12")).getByTestId("pr-chip")).toHaveTextContent("#12");
    expect(within(screen.getByTestId("artifact-group-12")).getByTestId("pr-chip")).toHaveTextContent("Merged");
    expect(within(screen.getByTestId("artifact-group-11")).queryByTestId("pr-chip")).toBeNull();
    expect(within(screen.getByTestId("artifact-group-11")).getByRole("link", { name: /Open #11 acme\/app on GitHub/ })).toBeInTheDocument();
  });

  it("group should cap at 50 newest items behind Show all", () => {
    const commits = Array.from({ length: 55 }, (_, i) => commitRec(at(10, 1) + i * 1000, `${String(i).padStart(2, "0")}${"a".repeat(38)}`));
    render(<ArtifactsTab session={grouped([prRec(at(10, 0), 11), ...commits])} />);
    const group = screen.getByTestId("artifact-group-11");
    expect(within(group).getAllByTestId("artifact-item-commit")).toHaveLength(50);
    fireEvent.click(within(group).getByRole("button", { name: "Show all (55)" }));
    expect(within(group).getAllByTestId("artifact-item-commit")).toHaveLength(55);
  });

  it("copy button should copy the full value and announce it", async () => {
    const writeText = jest.fn().mockResolvedValue(undefined);
    setClipboard({ writeText });
    render(<ArtifactsTab session={grouped([prRec(at(10, 0), 11), commitRec(at(10, 30), "1a2b3c4d5e6f")])} />);
    fireEvent.click(screen.getByRole("button", { name: "Copy commit SHA 1a2b3c4" }));
    expect(writeText).toHaveBeenCalledWith("1a2b3c4d5e6f");
    await waitFor(() => expect(mockAnnounce).toHaveBeenCalledWith("Copied commit 1a2b3c4", "polite", "artifact-copy"));
  });

  it("copy button should announce a failure when the clipboard is missing or rejects", async () => {
    const original = navigator.clipboard;
    const session = () => grouped([prRec(at(10, 0), 11), commitRec(at(10, 30), "1a2b3c4d5e6f")]);
    try {
      setClipboard(undefined);
      const { unmount } = render(<ArtifactsTab session={session()} />);
      fireEvent.click(screen.getByRole("button", { name: "Copy commit SHA 1a2b3c4" }));
      await waitFor(() => expect(mockAnnounce).toHaveBeenCalledWith("Couldn't copy commit", "assertive", "artifact-copy"));
      unmount();

      mockAnnounce.mockClear();
      setClipboard({ writeText: jest.fn().mockRejectedValue(new Error("denied")) });
      render(<ArtifactsTab session={session()} />);
      fireEvent.click(screen.getByRole("button", { name: "Copy commit SHA 1a2b3c4" }));
      await waitFor(() => expect(mockAnnounce).toHaveBeenCalledWith("Couldn't copy commit", "assertive", "artifact-copy"));
    } finally {
      setClipboard(original);
    }
  });
});
