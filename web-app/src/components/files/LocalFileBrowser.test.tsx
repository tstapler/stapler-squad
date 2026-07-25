/**
 * Tests for LocalFileBrowser.
 *
 * Covers:
 *  1. Directories (including symlinked ones, which the backend reports the same
 *     way as regular directories once resolved) navigate on click — regression
 *     for the snake_case/camelCase field mismatch that made every entry look
 *     like a non-navigable file.
 *  2. The filter box narrows the already-fetched entries with no extra fetch.
 *  3. The truncation notice reads the `has_more`/`total` fields the backend
 *     actually sends, not the stale `truncated` field.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { LocalFileBrowser, serveUrl } from "./LocalFileBrowser";

jest.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams("path=/home/user"),
  useRouter: () => ({ push: jest.fn(), replace: jest.fn() }),
}));

// RepoPathInput pulls in useSessionRepoPaths (Redux) and usePathCompletions (RPC).
// Stub both so this test doesn't need a Redux store or ConnectRPC transport.
jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: () => [],
}));
jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));

const createSession = jest.fn();
jest.mock("@/lib/hooks/useSessionService", () => ({
  useSessionService: () => ({ createSession }),
}));

jest.mock("@/lib/contexts/AnalyticsContext", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

function mockListingOnce(body: unknown) {
  (global.fetch as jest.Mock).mockResolvedValueOnce({
    ok: true,
    json: () => Promise.resolve(body),
  });
}

describe("LocalFileBrowser — navigation", () => {
  beforeEach(() => {
    global.fetch = jest.fn();
  });

  it("descends into a directory entry on click, including a resolved symlinked directory", async () => {
    mockListingOnce({
      path: "/home/user",
      parent: "/home",
      total: 2,
      has_more: false,
      entries: [
        { name: "real-dir", path: "/home/user/real-dir", is_dir: true, size: 0 },
        { name: "link-dir", path: "/home/user/link-dir", is_dir: true, size: 0 },
      ],
    });
    mockListingOnce({
      path: "/home/user/link-dir",
      parent: "/home/user",
      total: 0,
      has_more: false,
      entries: [],
    });

    render(<LocalFileBrowser />);

    const linkEntry = await screen.findByText("link-dir/");
    fireEvent.click(linkEntry);

    await waitFor(() => expect(global.fetch).toHaveBeenCalledTimes(2));
    expect(global.fetch).toHaveBeenLastCalledWith(
      expect.stringContaining(encodeURIComponent("/home/user/link-dir")),
      expect.anything()
    );
  });
});

describe("LocalFileBrowser — filter", () => {
  beforeEach(() => {
    global.fetch = jest.fn();
  });

  it("narrows entries client-side without an extra fetch", async () => {
    mockListingOnce({
      path: "/home/user",
      parent: "/home",
      total: 2,
      has_more: false,
      entries: [
        { name: "apple.txt", path: "/home/user/apple.txt", is_dir: false, size: 1 },
        { name: "banana.txt", path: "/home/user/banana.txt", is_dir: false, size: 1 },
      ],
    });

    render(<LocalFileBrowser />);

    await screen.findByText("apple.txt");
    expect(screen.getByText("banana.txt")).toBeInTheDocument();

    fireEvent.change(screen.getByTestId("file-browser-filter-input"), {
      target: { value: "app" },
    });

    expect(screen.getByText("apple.txt")).toBeInTheDocument();
    expect(screen.queryByText("banana.txt")).not.toBeInTheDocument();
    expect(global.fetch).toHaveBeenCalledTimes(1);
  });
});

describe("serveUrl — special-character filenames", () => {
  it.each([
    "/home/user/weird name #1.txt",
    "/home/user/what?.txt",
    "/home/user/résumé 日本語.pdf",
  ])("round-trips %s through encode/decode", (absPath) => {
    const url = serveUrl(absPath);
    const decodedPath = decodeURIComponent(url.replace("/api/local/serve", ""));
    expect(decodedPath).toBe(absPath);
  });
});

describe("LocalFileBrowser — truncation notice", () => {
  beforeEach(() => {
    global.fetch = jest.fn();
  });

  it("shows an accurate notice sourced from has_more/total, not a hardcoded count", async () => {
    mockListingOnce({
      path: "/home/user",
      parent: "/home",
      total: 2001,
      has_more: true,
      entries: Array.from({ length: 2000 }, (_, i) => ({
        name: `file-${i}`,
        path: `/home/user/file-${i}`,
        is_dir: false,
        size: 0,
      })),
    });

    render(<LocalFileBrowser />);

    const notice = await screen.findByTestId("file-browser-truncation-notice");
    expect(notice).toHaveTextContent("Showing first 2000 of 2001 entries");
  });
});
