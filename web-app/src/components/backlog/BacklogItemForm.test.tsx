/**
 * Tests for BacklogItemForm — focused on the repo-path UX fix.
 *
 * Covers:
 *  1. Repository Path field renders its generic hint text
 *  2. Skip planning / skip review gate checkboxes render plain-language help text
 *  3. Typing a GitHub URL into Repository Path shows the "Will clone" confirmation
 *  4. Submitting with a GitHub URL in Repository Path shows a "Cloning repository…" busy label
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { BacklogItemForm } from "./BacklogItemForm";

// RepoPathInput uses useSessionRepoPaths (Redux) and usePathCompletions (RPC).
// Stub both so tests don't need a Redux store or ConnectRPC transport.
jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: () => [],
}));

jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));

describe("BacklogItemForm — Repository Path help", () => {
  it("renders the generic repo-path hint", () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    expect(
      screen.getByText("Local path to your clone, or a GitHub URL — we'll clone it for you.")
    ).toBeInTheDocument();
  });

  it("shows a clone confirmation when a GitHub URL is entered", () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    fireEvent.change(screen.getByTestId("backlog-repo-path-input"), {
      target: { value: "https://github.com/facebook/react" },
    });

    expect(screen.getByTestId("repo-path-github-hint")).toHaveTextContent(
      "Will clone facebook/react"
    );
  });
});

describe("BacklogItemForm — checkbox help text", () => {
  it("explains skip planning phase in plain language", () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    expect(
      screen.getByText("Go straight to triage without a separate planning pass.")
    ).toBeInTheDocument();
  });

  it("explains skip review gate in plain language", () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    expect(
      screen.getByText("Mark work done without an automated review pass first.")
    ).toBeInTheDocument();
  });
});

describe("BacklogItemForm — cloning busy state", () => {
  it("shows 'Cloning repository…' instead of 'Saving…' while submitting a GitHub URL", async () => {
    let resolveSubmit: () => void = () => {};
    const onSubmit = jest.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveSubmit = resolve;
        })
    );

    render(<BacklogItemForm onSubmit={onSubmit} onCancel={jest.fn()} />);

    fireEvent.change(screen.getByTestId("backlog-title-input"), {
      target: { value: "Clone and triage this repo" },
    });
    fireEvent.change(screen.getByTestId("backlog-repo-path-input"), {
      target: { value: "https://github.com/facebook/react" },
    });
    fireEvent.click(screen.getByTestId("backlog-form-submit"));

    await waitFor(() =>
      expect(screen.getByTestId("backlog-form-submit")).toHaveTextContent("Cloning repository…")
    );

    resolveSubmit();
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
  });
});

describe("BacklogItemForm — description write/preview", () => {
  it("renders markdown in the preview tab and shows raw text in the write tab", () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    fireEvent.change(screen.getByTestId("backlog-description-input"), {
      target: { value: "**bold text**" },
    });

    fireEvent.click(screen.getByTestId("backlog-description-tab-preview"));
    expect(screen.getByText("bold text").tagName).toBe("STRONG");

    fireEvent.click(screen.getByTestId("backlog-description-tab-write"));
    expect(screen.getByTestId("backlog-description-input")).toHaveValue("**bold text**");
  });

  it("renders a link with correct href in preview", () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    fireEvent.change(screen.getByTestId("backlog-description-input"), {
      target: { value: "see [logo](https://example.com/logo.png)" },
    });
    fireEvent.click(screen.getByTestId("backlog-description-tab-preview"));

    const link = screen.getByRole("link", { name: "logo" });
    expect(link).toHaveAttribute("href", "https://example.com/logo.png");
  });

  it("never executes injected script tags in preview", () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    fireEvent.change(screen.getByTestId("backlog-description-input"), {
      target: { value: "<script>window.__pwned = true;</script>" },
    });
    fireEvent.click(screen.getByTestId("backlog-description-tab-preview"));

    expect(document.querySelector("script")).not.toBeInTheDocument();
    expect((window as unknown as { __pwned?: boolean }).__pwned).toBeUndefined();
  });
});

describe("BacklogItemForm — image attachment upload", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it("uploads an image and inserts a markdown image reference", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ path: "/home/user/.stapler-squad/backlog-attachments/1-screenshot.png", filename: "1-screenshot.png" }),
    });

    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    const file = new File(["fake-bytes"], "screenshot.png", { type: "image/png" });
    fireEvent.change(screen.getByTestId("backlog-attach-image-input"), {
      target: { files: [file] },
    });

    await waitFor(() =>
      expect(
        (screen.getByTestId("backlog-description-input") as HTMLTextAreaElement).value
      ).toContain("![1-screenshot.png]")
    );
    expect(global.fetch).toHaveBeenCalledWith(
      expect.stringContaining("/v1/upload-backlog-attachment"),
      expect.objectContaining({ method: "POST" })
    );
  });

  it("rejects a non-image file client-side without calling fetch", () => {
    global.fetch = jest.fn();
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    const file = new File(["hello"], "notes.txt", { type: "text/plain" });
    fireEvent.change(screen.getByTestId("backlog-attach-image-input"), {
      target: { files: [file] },
    });

    expect(screen.getByTestId("backlog-attach-image-error")).toHaveTextContent(
      "Only image files can be attached."
    );
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it("rejects an oversized image client-side without calling fetch", () => {
    global.fetch = jest.fn();
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    const bigFile = new File([new Uint8Array(11 * 1024 * 1024)], "big.png", { type: "image/png" });
    fireEvent.change(screen.getByTestId("backlog-attach-image-input"), {
      target: { files: [bigFile] },
    });

    expect(screen.getByTestId("backlog-attach-image-error")).toHaveTextContent(
      "Image is too large (max 10 MB)."
    );
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it("shows a server-side rejection message on a 415 response", async () => {
    global.fetch = jest.fn().mockResolvedValue({ ok: false, status: 415 });
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    const file = new File(["fake-bytes"], "photo.png", { type: "image/png" });
    fireEvent.change(screen.getByTestId("backlog-attach-image-input"), {
      target: { files: [file] },
    });

    await waitFor(() =>
      expect(screen.getByTestId("backlog-attach-image-error")).toHaveTextContent(
        "Unsupported image type"
      )
    );
  });
});
