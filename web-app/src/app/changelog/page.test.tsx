import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import ChangelogPage from "./page";

jest.mock("@/components/changelog/changelog.css", () => new Proxy({}, { get: (_t, p) => (typeof p === "string" ? p : "") }));
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlags: () => ({ flags: { changelog: true }, isLoading: false }),
}));

const changelog = {
  current_version: "2.4.0",
  is_source_build: false,
  releases: [
    { version: "2.5.0", date: "2026-10-14", sections: [{ title: "Features", entries: [{ scope: "web", text: "new thing", prs: [12] }] }] },
    { version: "2.4.0", date: "2026-10-13", sections: [{ title: "Bug Fixes", entries: [{ text: "fixed it" }] }] },
  ],
};

function mockFetch(body: unknown, ok = true) {
  global.fetch = jest.fn((url: string) =>
    Promise.resolve({
      ok: ok || url !== "/api/changelog",
      status: ok ? 200 : 500,
      json: () => Promise.resolve(url === "/api/update-check" ? { enabled: false, is_outdated: false, is_source_build: false, current_version: "2.4.0" } : body),
    })
  ) as unknown as typeof fetch;
}

describe("ChangelogPage", () => {
  beforeAll(() => {
    Element.prototype.scrollIntoView = jest.fn();
  });

  it("renders releases and marks the running version", async () => {
    mockFetch(changelog);
    render(<ChangelogPage />);
    expect(await screen.findByText("v2.5.0")).toBeInTheDocument();
    const current = screen.getByTestId("release-2.4.0");
    expect(current).toHaveAttribute("aria-current", "true");
    expect(current).toHaveTextContent("You are here");
    expect(screen.getByText("fixed it")).toBeInTheDocument(); // current release expanded
    expect(screen.queryByText("new thing")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /v2\.5\.0/ }));
    expect(screen.getByText("new thing")).toBeInTheDocument();
    expect(screen.getByText("(#12)")).toBeInTheDocument();
  });

  it("shows the Source build badge with branch and commit", async () => {
    mockFetch({ ...changelog, current_version: "dev", branch: "feature-x", commit: "abc1234", is_source_build: true });
    render(<ChangelogPage />);
    expect(await screen.findByTestId("source-build-badge")).toHaveTextContent("Source build");
    expect(screen.getByTestId("build-provenance")).toHaveTextContent("abc1234");
    expect(screen.getByTestId("build-provenance")).toHaveTextContent("feature-x");
  });

  it("shows an alert when the API fails", async () => {
    mockFetch({}, false);
    render(<ChangelogPage />);
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(/Couldn.t load/));
  });
});
