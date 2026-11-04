import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { create, type MessageInitShape } from "@bufbuild/protobuf";
import { ArtifactsTab, SCAN_SLOW_AFTER_MS } from "../ArtifactsTab";
import { SessionArtifactsSchema } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";

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
}): Session["artifacts"] {
  return a as unknown as Session["artifacts"];
}

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

    describe("slow scan escape (UX-S7-4)", () => {
      beforeEach(() => jest.useFakeTimers());
      afterEach(() => jest.useRealTimers());

      it("ArtifactsTab_should_offerRescanAndCancelWaiting_When_scanningOver60s", () => {
        const onRescan = jest.fn();
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
