import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { ArtifactsTab } from "../ArtifactsTab";
import { SessionArtifactsSchema } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";

type ArtifactsInit = Parameters<typeof create<typeof SessionArtifactsSchema>>[1];

function makeSession(artifactsFields?: ArtifactsInit): Session {
  const artifacts = artifactsFields != null
    ? create(SessionArtifactsSchema, artifactsFields)
    : undefined;
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
    const link = screen.getByRole("link", { name: /…$/ });
    expect(link).toBeInTheDocument();
    expect(link.textContent!.length).toBeLessThan(longURL.length);
    expect(link).toHaveAttribute("href", longURL);
  });

  it("ArtifactsTab_should_addSecurityAttrsToExternalLinks", () => {
    render(
      <ArtifactsTab
        session={makeSession({
          prUrls: ["https://github.com/owner/repo/pull/42"],
          commitShas: [],
          externalUrls: [],
        })}
      />
    );
    const link = screen.getByRole("link", { name: "owner/repo#42" });
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("ArtifactsTab_should_renderMultiplePRLinks", () => {
    render(
      <ArtifactsTab
        session={makeSession({
          prUrls: [
            "https://github.com/owner/repo/pull/1",
            "https://github.com/owner/repo/pull/2",
          ],
          commitShas: [],
          externalUrls: [],
        })}
      />
    );
    expect(screen.getByText("owner/repo#1")).toBeInTheDocument();
    expect(screen.getByText("owner/repo#2")).toBeInTheDocument();
  });

  it("ArtifactsTab_should_renderRawURLForMalformedPRURL", () => {
    // A URL without /pull/ should render as-is (parsePRDisplay returns the raw URL)
    render(
      <ArtifactsTab
        session={makeSession({
          prUrls: ["https://github.com/owner/repo/issues/5"],
          commitShas: [],
          externalUrls: [],
        })}
      />
    );
    // parsePRDisplay returns the raw URL when it can't parse as a PR URL
    expect(screen.getByRole("link")).toBeInTheDocument();
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
