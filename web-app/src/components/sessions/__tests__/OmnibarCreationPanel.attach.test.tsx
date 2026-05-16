// @feature omnibar-image-attachment
/**
 * Tests for the OmnibarCreationPanel image attachment UI (US-2).
 * Covers attach button, thumbnail previews, file limit, remove, and callbacks.
 */

import React from "react";
import { render, screen, fireEvent, act, waitFor } from "@testing-library/react";
import { OmnibarCreationPanel } from "../OmnibarCreationPanel";
import type { OmnibarCreationPanelProps } from "../OmnibarCreationPanel";
import type { OmnibarFormState } from "../Omnibar";

// ── Baseline form state ───────────────────────────────────────────────────────

const DEFAULT_FORM_STATE: OmnibarFormState = {
  sessionName: "test-session",
  branch: "",
  program: "claude",
  category: "",
  autoYes: false,
  useTitleAsBranch: true,
  sessionType: "new_worktree",
  existingWorktree: "",
  workingDir: "",
  parentDir: "",
  projectName: "",
  newProjectSessionType: "new_worktree",
  firstPrompt: "",
  createIfMissing: false,
};

function buildProps(overrides: Partial<OmnibarCreationPanelProps> = {}): OmnibarCreationPanelProps {
  return {
    formState: DEFAULT_FORM_STATE,
    setFormField: jest.fn(),
    onSubmit: jest.fn(),
    onCancel: jest.fn(),
    worktrees: [],
    isSubmitting: false,
    canSubmit: true,
    error: null,
    showAdvanced: false,
    onToggleAdvanced: jest.fn(),
    uploadBaseUrl: "/api",
    onAttachedImagesChange: jest.fn(),
    ...overrides,
  };
}

// Track object URLs created/revoked.
const createdObjectUrls: string[] = [];
const revokedObjectUrls: string[] = [];

// Save originals so they can be restored — jest.restoreAllMocks() won't revert
// direct property assignments on global.URL.
const _origCreateObjectURL = global.URL.createObjectURL;
const _origRevokeObjectURL = global.URL.revokeObjectURL;

// ── Setup / Teardown ──────────────────────────────────────────────────────────

beforeEach(() => {
  jest.clearAllMocks();
  createdObjectUrls.length = 0;
  revokedObjectUrls.length = 0;

  global.fetch = jest.fn();

  // Mock URL.createObjectURL and URL.revokeObjectURL
  global.URL.createObjectURL = jest.fn((file: File) => {
    const url = `blob:${file.name}`;
    createdObjectUrls.push(url);
    return url;
  });
  global.URL.revokeObjectURL = jest.fn((url: string) => {
    revokedObjectUrls.push(url);
  });

  jest.spyOn(console, "log").mockImplementation(() => {});
  jest.spyOn(console, "warn").mockImplementation(() => {});
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  jest.restoreAllMocks();
  global.URL.createObjectURL = _origCreateObjectURL;
  global.URL.revokeObjectURL = _origRevokeObjectURL;
});

// ── FT3-01: Renders "Attach image" button ─────────────────────────────────────

describe("FT3-01: renders 'Attach image' button in creation panel", () => {
  it("attach button is present in the document", () => {
    render(<OmnibarCreationPanel {...buildProps()} />);
    expect(screen.getByText(/Attach image/i)).toBeInTheDocument();
  });
});

// ── FT3-02: Attach button click triggers hidden file input ───────────────────

describe("FT3-02: attach button click triggers hidden file input", () => {
  it("clicking the attach button calls .click() on the hidden input", () => {
    const clickSpy = jest.spyOn(HTMLInputElement.prototype, "click");
    render(<OmnibarCreationPanel {...buildProps()} />);

    const btn = screen.getByText(/Attach image/i);
    fireEvent.click(btn);

    expect(clickSpy).toHaveBeenCalledTimes(1);
    clickSpy.mockRestore();
  });
});

// ── FT3-03: Selecting 1 file uploads and shows thumbnail ─────────────────────

describe("FT3-03: selecting 1 file uploads and shows thumbnail", () => {
  it("thumbnail appears after successful upload", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ path: "/tmp/uploads/img1.jpg" }),
    });

    render(<OmnibarCreationPanel {...buildProps()} />);

    const fileInputs = document.querySelectorAll('input[type="file"][accept="image/*"]');
    const input = fileInputs[0] as HTMLInputElement;
    expect(input).toBeTruthy();

    const file = new File(["fake"], "img1.jpg", { type: "image/jpeg" });
    Object.defineProperty(input, "files", { value: [file], configurable: true });

    await act(async () => {
      fireEvent.change(input);
    });

    await waitFor(() => {
      const imgs = screen.getAllByRole("img");
      const thumb = imgs.find((img) => img.getAttribute("src")?.includes("blob:"));
      expect(thumb).toBeTruthy();
    });
  });
});

// ── FT3-04: Selecting 3 files uploads all and disables button ────────────────

describe("FT3-04: selecting 3 files uploads all and disables attach button", () => {
  it("attach button is disabled after 3 images are attached", async () => {
    for (let i = 0; i < 3; i++) {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        json: async () => ({ path: `/tmp/uploads/img${i}.jpg` }),
      });
    }

    render(<OmnibarCreationPanel {...buildProps()} />);

    const input = document.querySelector('input[type="file"][accept="image/*"]') as HTMLInputElement;

    const files = [
      new File(["a"], "a.jpg", { type: "image/jpeg" }),
      new File(["b"], "b.jpg", { type: "image/jpeg" }),
      new File(["c"], "c.jpg", { type: "image/jpeg" }),
    ];
    Object.defineProperty(input, "files", { value: files, configurable: true });

    await act(async () => {
      fireEvent.change(input);
    });

    await waitFor(() => {
      const imgs = screen.getAllByRole("img").filter((img) =>
        img.getAttribute("src")?.startsWith("blob:")
      );
      expect(imgs).toHaveLength(3);
    });

    // Attach button should now be disabled
    const attachBtn = screen.getByText(/Attach image/i);
    expect(attachBtn).toBeDisabled();
  });
});

// ── FT3-05: Selecting 4 files only uploads first 3 ───────────────────────────

describe("FT3-05: selecting 4 files only uploads first 3", () => {
  it("fetch is called only 3 times when 4 files are selected", async () => {
    for (let i = 0; i < 4; i++) {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        json: async () => ({ path: `/tmp/uploads/img${i}.jpg` }),
      });
    }

    render(<OmnibarCreationPanel {...buildProps()} />);

    const input = document.querySelector('input[type="file"][accept="image/*"]') as HTMLInputElement;

    const files = [
      new File(["a"], "a.jpg", { type: "image/jpeg" }),
      new File(["b"], "b.jpg", { type: "image/jpeg" }),
      new File(["c"], "c.jpg", { type: "image/jpeg" }),
      new File(["d"], "d.jpg", { type: "image/jpeg" }),
    ];
    Object.defineProperty(input, "files", { value: files, configurable: true });

    await act(async () => {
      fireEvent.change(input);
    });

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledTimes(3);
    });

    // Only 3 thumbnails
    const imgs = screen.getAllByRole("img").filter((img) =>
      img.getAttribute("src")?.startsWith("blob:")
    );
    expect(imgs).toHaveLength(3);
  });
});

// ── FT3-06: Removing thumbnail revokes object URL ────────────────────────────

describe("FT3-06: removing thumbnail revokes object URL and removes from list", () => {
  it("clicking remove button revokes the preview URL and removes the thumbnail", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ path: "/tmp/uploads/img1.jpg" }),
    });

    render(<OmnibarCreationPanel {...buildProps()} />);

    const input = document.querySelector('input[type="file"][accept="image/*"]') as HTMLInputElement;
    const file = new File(["fake"], "img1.jpg", { type: "image/jpeg" });
    Object.defineProperty(input, "files", { value: [file], configurable: true });

    await act(async () => {
      fireEvent.change(input);
    });

    await waitFor(() => {
      expect(screen.getAllByRole("img").some((img) => img.getAttribute("src")?.startsWith("blob:"))).toBe(true);
    });

    // Click the remove button
    const removeBtn = screen.getByRole("button", { name: /remove/i });
    await act(async () => {
      fireEvent.click(removeBtn);
    });

    // Object URL should be revoked
    expect(URL.revokeObjectURL).toHaveBeenCalledTimes(1);

    // Thumbnail should be gone
    const imgs = screen.queryAllByRole("img").filter((img) =>
      img.getAttribute("src")?.startsWith("blob:")
    );
    expect(imgs).toHaveLength(0);
  });
});

// ── FT3-07: Upload failure shows error message ───────────────────────────────

describe("FT3-07: upload failure shows error message", () => {
  it("shows 'Upload failed' when fetch returns non-ok status", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: false,
      status: 500,
    });

    render(<OmnibarCreationPanel {...buildProps()} />);

    const input = document.querySelector('input[type="file"][accept="image/*"]') as HTMLInputElement;
    const file = new File(["x"], "photo.jpg", { type: "image/jpeg" });
    Object.defineProperty(input, "files", { value: [file], configurable: true });

    await act(async () => {
      fireEvent.change(input);
    });

    await waitFor(() => {
      expect(screen.getByText(/Upload failed/i)).toBeInTheDocument();
    });
  });
});

// ── FT3-08: onAttachedImagesChange called with correct paths ─────────────────

describe("FT3-08: onAttachedImagesChange called with correct paths when images added", () => {
  it("callback is called with the paths of all uploaded images", async () => {
    const onAttachedImagesChange = jest.fn();

    (global.fetch as jest.Mock)
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ path: "/tmp/img1.jpg" }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ path: "/tmp/img2.jpg" }),
      });

    render(<OmnibarCreationPanel {...buildProps({ onAttachedImagesChange })} />);

    const input = document.querySelector('input[type="file"][accept="image/*"]') as HTMLInputElement;
    const files = [
      new File(["a"], "img1.jpg", { type: "image/jpeg" }),
      new File(["b"], "img2.jpg", { type: "image/jpeg" }),
    ];
    Object.defineProperty(input, "files", { value: files, configurable: true });

    await act(async () => {
      fireEvent.change(input);
    });

    await waitFor(() => {
      const calls = (onAttachedImagesChange as jest.Mock).mock.calls;
      const lastCall = calls[calls.length - 1][0] as string[];
      expect(lastCall).toContain("/tmp/img1.jpg");
      expect(lastCall).toContain("/tmp/img2.jpg");
    });
  });
});

// ── FT3-09: onAttachedImagesChange called with empty array when all removed ───

describe("FT3-09: onAttachedImagesChange called with empty array when all removed", () => {
  it("callback is called with [] after the only image is removed", async () => {
    const onAttachedImagesChange = jest.fn();

    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ path: "/tmp/img1.jpg" }),
    });

    render(<OmnibarCreationPanel {...buildProps({ onAttachedImagesChange })} />);

    const input = document.querySelector('input[type="file"][accept="image/*"]') as HTMLInputElement;
    const file = new File(["a"], "img1.jpg", { type: "image/jpeg" });
    Object.defineProperty(input, "files", { value: [file], configurable: true });

    await act(async () => {
      fireEvent.change(input);
    });

    await waitFor(() => {
      const calls = (onAttachedImagesChange as jest.Mock).mock.calls;
      const lastCall = calls[calls.length - 1][0] as string[];
      expect(lastCall).toContain("/tmp/img1.jpg");
    });

    // Remove the image
    const removeBtn = screen.getByRole("button", { name: /remove/i });
    await act(async () => {
      fireEvent.click(removeBtn);
    });

    await waitFor(() => {
      const calls = (onAttachedImagesChange as jest.Mock).mock.calls;
      const lastCall = calls[calls.length - 1][0] as string[];
      expect(lastCall).toEqual([]);
    });
  });
});
