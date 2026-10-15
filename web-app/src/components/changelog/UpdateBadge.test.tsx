import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { UpdateBadge } from "./UpdateBadge";

jest.mock("./changelog.css", () => new Proxy({}, { get: (_t, p) => (typeof p === "string" ? p : "") }));
let mockFlags: Record<string, boolean> = { changelog: true };
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlags: () => ({ flags: mockFlags, isLoading: false }),
}));

const respond = (body: object) => {
  global.fetch = jest.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(body) }) as unknown as typeof fetch;
};

describe("UpdateBadge", () => {
  it("links to the release when outdated", async () => {
    respond({ enabled: true, is_outdated: true, is_source_build: false, current_version: "2.4.0", latest_version: "2.5.0", latest_url: "https://example.com/r" });
    render(<UpdateBadge />);
    const a = await screen.findByTestId("update-badge");
    expect(a).toHaveAttribute("href", "https://example.com/r");
  });

  it("renders nothing for a source build", async () => {
    respond({ enabled: true, is_outdated: true, is_source_build: true, current_version: "dev", latest_version: "2.5.0" });
    render(<UpdateBadge />);
    await waitFor(() => expect(global.fetch).toHaveBeenCalled());
    expect(screen.queryByTestId("update-badge")).not.toBeInTheDocument();
  });

  it("does not fetch or render when the changelog flag is off", async () => {
    mockFlags = { changelog: false };
    respond({ enabled: true, is_outdated: true, is_source_build: false, latest_version: "2.5.0" });
    render(<UpdateBadge />);
    expect(global.fetch).not.toHaveBeenCalled();
    expect(screen.queryByTestId("update-badge")).not.toBeInTheDocument();
    mockFlags = { changelog: true };
  });
});
