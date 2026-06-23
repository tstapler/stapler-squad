import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { ArtifactsTab } from "../ArtifactsTab";
import type { Session } from "@/gen/session/v1/types_pb";

// Build a minimal Session-like object for testing — avoids importing full protobuf runtime.
function makeSession(artifacts?: Session["artifacts"]): Session {
  return { artifacts } as unknown as Session;
}

describe("ArtifactsTab", () => {
  it("ArtifactsTab_should_showEmptyState_When_artifactsIsNull", () => {
    render(<ArtifactsTab session={makeSession(undefined)} />);
    expect(screen.getByText(/Extraction pending/)).toBeInTheDocument();
  });

  it("ArtifactsTab_should_showNoArtifacts_When_artifactsIsEmptyArrays", () => {
    render(
      <ArtifactsTab
        session={makeSession({ prUrls: [], commitShas: [], externalUrls: [] })}
      />
    );
    expect(screen.getByText(/No artifacts found/)).toBeInTheDocument();
  });

  it("ArtifactsTab_should_renderPRLinks_When_artifactsHasPRURLs", () => {
    render(
      <ArtifactsTab
        session={makeSession({
          prUrls: ["https://github.com/owner/repo/pull/42"],
          commitShas: [],
          externalUrls: [],
        })}
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
        session={makeSession({
          prUrls: [],
          commitShas: [],
          externalUrls: [longURL],
        })}
      />
    );
    // External URLs are behind a disclosure toggle — click to expand.
    fireEvent.click(screen.getByRole("button", { name: /Show 1 external URL/ }));
    expect(screen.getByText(/…$/)).toBeInTheDocument();
  });

  it("ArtifactsTab_should_renderCommitSHAs_When_artifactsHasCommits", () => {
    const sha = "abc123def456abc123def456abc123def456abc1";
    render(
      <ArtifactsTab
        session={makeSession({
          prUrls: [],
          commitShas: [sha],
          externalUrls: [],
        })}
      />
    );
    // Renders the shortened 7-char prefix in a <code> element.
    expect(screen.getByText("abc123d")).toBeInTheDocument();
  });
});
