// @feature terminal-image-upload
/**
 * Tests for the terminal toolbar image upload button (US-1).
 * These tests verify the hidden file input, upload flow, error handling, and
 * the trailing-space insertion of the returned path into handleTerminalData.
 */

import React from "react";
import { render, screen, fireEvent, act, waitFor } from "@testing-library/react";

// ── Mocks ─────────────────────────────────────────────────────────────────────

const mockXtermHandle = {
  terminal: null as null,
  fit: jest.fn(),
  write: jest.fn(),
  writeln: jest.fn(),
  clear: jest.fn(),
  focus: jest.fn(),
  search: jest.fn().mockReturnValue(false),
  searchNext: jest.fn().mockReturnValue(false),
  searchPrevious: jest.fn().mockReturnValue(false),
};

jest.mock("../XtermTerminal", () => {
  const React = require("react");
  const XtermTerminal = React.forwardRef((props: any, ref: any) => {
    React.useImperativeHandle(ref, () => mockXtermHandle);
    return React.createElement("div", { "data-testid": "mock-xterm" });
  });
  XtermTerminal.displayName = "XtermTerminal";
  return { XtermTerminal };
});

jest.mock("@/lib/hooks/useTerminalStream", () => ({
  useTerminalStream: jest.fn(),
}));

jest.mock("@/lib/terminal/TerminalDimensionCache", () => ({
  getCachedDimensions: jest.fn().mockReturnValue(null),
  saveDimensions: jest.fn(),
}));

jest.mock("@/lib/terminal/TerminalStreamManager", () => ({
  TerminalStreamManager: jest.fn().mockImplementation(() => ({
    setOnFirstOutput: jest.fn(),
    installDebugMonitor: jest.fn(),
    writeInitialContent: jest.fn().mockResolvedValue(undefined),
    write: jest.fn(),
    cleanup: jest.fn(),
    updateSendFlowControl: jest.fn(),
  })),
}));

jest.mock("@/lib/telemetry", () => ({ track: jest.fn() }));

// ── Imports (after mocks) ──────────────────────────────────────────────────────

// eslint-disable-next-line import/first
import { TerminalOutput } from "../TerminalOutput";
// eslint-disable-next-line import/first
import { useTerminalStream } from "@/lib/hooks/useTerminalStream";

// ── Helpers ───────────────────────────────────────────────────────────────────

function makeStreamMock(overrides = {}) {
  return {
    isConnected: false,
    error: null,
    connect: jest.fn(),
    disconnect: jest.fn(),
    sendInput: jest.fn(),
    sendInputWithEcho: jest.fn().mockReturnValue(BigInt(0)),
    resize: jest.fn(),
    scrollbackLoaded: false,
    requestScrollback: jest.fn(),
    sendFlowControl: jest.fn(),
    getIsApplyingState: jest.fn().mockReturnValue(false),
    sspNegotiated: false,
    startRecording: jest.fn(),
    stopRecording: jest.fn(),
    ...overrides,
  };
}

function renderTerminal(sessionId = "session-abc", baseUrl = "/api") {
  return render(
    <TerminalOutput sessionId={sessionId} baseUrl={baseUrl} isVisible={false} />
  );
}

// ── Setup / Teardown ──────────────────────────────────────────────────────────

beforeEach(() => {
  jest.clearAllMocks();
  (useTerminalStream as jest.Mock).mockReturnValue(makeStreamMock());
  global.fetch = jest.fn();
  jest.spyOn(console, "log").mockImplementation(() => {});
  jest.spyOn(console, "warn").mockImplementation(() => {});
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  jest.restoreAllMocks();
});

// ── FT2-01: Renders upload button ────────────────────────────────────────────

describe("FT2-01: renders upload button in toolbar", () => {
  it("upload button is present in the document", () => {
    renderTerminal();
    const btn = screen.getByRole("button", { name: /attach image/i });
    expect(btn).toBeInTheDocument();
    expect(btn).not.toBeDisabled();
  });
});

// ── FT2-02: Button click triggers hidden input click ─────────────────────────

describe("FT2-02: upload button click triggers hidden file input click", () => {
  it("clicking upload button calls .click() on the hidden input", () => {
    renderTerminal();
    const clickSpy = jest.spyOn(HTMLInputElement.prototype, "click");

    const btn = screen.getByRole("button", { name: /attach image/i });
    fireEvent.click(btn);

    expect(clickSpy).toHaveBeenCalledTimes(1);
    clickSpy.mockRestore();
  });

  it("hidden file input has accept='image/*' and no capture attribute", () => {
    renderTerminal();
    // The hidden input is aria-hidden but we can query by type
    const inputs = document.querySelectorAll('input[type="file"]');
    // There's one file input for the upload button
    const uploadInput = Array.from(inputs).find(
      (el) => el.getAttribute("accept") === "image/*"
    );
    expect(uploadInput).toBeTruthy();
    expect(uploadInput).not.toHaveAttribute("capture");
  });
});

// ── FT2-03: Successful upload inserts path + space ───────────────────────────

describe("FT2-03: successful upload calls handleTerminalData with path + space", () => {
  it("inserts '/workspace/uploads/123-photo.jpg ' into terminal on success", async () => {
    const sendInput = jest.fn();
    (useTerminalStream as jest.Mock).mockReturnValue(makeStreamMock({ sendInput }));

    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ path: "/workspace/uploads/123-photo.jpg", filename: "123-photo.jpg" }),
    });

    renderTerminal();

    const input = document.querySelector('input[type="file"][accept="image/*"]') as HTMLInputElement;
    expect(input).toBeTruthy();

    const file = new File(["fake-image-data"], "photo.jpg", { type: "image/jpeg" });
    Object.defineProperty(input, "files", { value: [file], configurable: true });

    await act(async () => {
      fireEvent.change(input);
    });

    await waitFor(() => {
      expect(sendInput).toHaveBeenCalledWith("/workspace/uploads/123-photo.jpg ");
    });
  });
});

// ── FT2-04: 413 error shows "File too large" ─────────────────────────────────

describe("FT2-04: upload 413 error shows 'File too large' message", () => {
  it("button text shows file too large on 413", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: false,
      status: 413,
      text: async () => "file too large",
    });

    renderTerminal();

    const input = document.querySelector('input[type="file"][accept="image/*"]') as HTMLInputElement;
    const file = new File(["x"], "big.jpg", { type: "image/jpeg" });
    Object.defineProperty(input, "files", { value: [file], configurable: true });

    await act(async () => {
      fireEvent.change(input);
    });

    await waitFor(() => {
      const btn = screen.getByRole("button", { name: /attach image/i });
      expect(btn.textContent).toMatch(/File too large/i);
    });
  });
});

// ── FT2-05: 400 error shows "Invalid image type" ─────────────────────────────

describe("FT2-05: upload 400 error shows 'Invalid image type' message", () => {
  it("button text shows invalid image type on 400", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: false,
      status: 400,
      text: async () => "unsupported",
    });

    renderTerminal();

    const input = document.querySelector('input[type="file"][accept="image/*"]') as HTMLInputElement;
    const file = new File(["x"], "doc.html", { type: "text/html" });
    Object.defineProperty(input, "files", { value: [file], configurable: true });

    await act(async () => {
      fireEvent.change(input);
    });

    await waitFor(() => {
      const btn = screen.getByRole("button", { name: /attach image/i });
      expect(btn.textContent).toMatch(/Invalid image type/i);
    });
  });
});

// ── FT2-06: Network error shows "Network error" ──────────────────────────────

describe("FT2-06: upload network error shows 'Network error' message", () => {
  it("button text shows network error when fetch rejects", async () => {
    (global.fetch as jest.Mock).mockRejectedValueOnce(new Error("Network failure"));

    renderTerminal();

    const input = document.querySelector('input[type="file"][accept="image/*"]') as HTMLInputElement;
    const file = new File(["x"], "photo.jpg", { type: "image/jpeg" });
    Object.defineProperty(input, "files", { value: [file], configurable: true });

    await act(async () => {
      fireEvent.change(input);
    });

    await waitFor(() => {
      const btn = screen.getByRole("button", { name: /attach image/i });
      expect(btn.textContent).toMatch(/Network error/i);
    });
  });
});

// ── FT2-07: Error message clears after 3 seconds ─────────────────────────────

describe("FT2-07: error message clears after 3 seconds", () => {
  it("upload error disappears after 3000ms", async () => {
    jest.useFakeTimers();

    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: false,
      status: 400,
      text: async () => "unsupported",
    });

    renderTerminal();

    const input = document.querySelector('input[type="file"][accept="image/*"]') as HTMLInputElement;
    const file = new File(["x"], "bad.jpg", { type: "image/jpeg" });
    Object.defineProperty(input, "files", { value: [file], configurable: true });

    await act(async () => {
      fireEvent.change(input);
    });

    // Error should be showing
    await waitFor(() => {
      const btn = screen.getByRole("button", { name: /attach image/i });
      expect(btn.textContent).toMatch(/Invalid image type/i);
    });

    // Advance timer past 3000ms
    await act(async () => {
      jest.advanceTimersByTime(3001);
    });

    // Error should be gone
    const btn = screen.getByRole("button", { name: /attach image/i });
    expect(btn.textContent).not.toMatch(/Invalid image type/i);

    jest.useRealTimers();
  });
});

// ── FT2-08: isUploading disables button and shows uploading text ──────────────

describe("FT2-08: isUploading disables button and shows uploading text", () => {
  it("button is disabled and shows 'Uploading...' during in-flight upload", async () => {
    // fetch never resolves
    (global.fetch as jest.Mock).mockReturnValueOnce(new Promise(() => {}));

    renderTerminal();

    const input = document.querySelector('input[type="file"][accept="image/*"]') as HTMLInputElement;
    const file = new File(["x"], "photo.jpg", { type: "image/jpeg" });
    Object.defineProperty(input, "files", { value: [file], configurable: true });

    act(() => {
      fireEvent.change(input);
    });

    await waitFor(() => {
      const btn = screen.getByRole("button", { name: /uploading image/i });
      expect(btn).toBeDisabled();
      expect(btn.textContent).toMatch(/Uploading/i);
    });
  });
});

// ── FT2-09: Same file can be selected twice ───────────────────────────────────

describe("FT2-09: same file can be selected twice (input value reset)", () => {
  it("fetch is called twice when the same file is selected twice", async () => {
    (global.fetch as jest.Mock)
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ path: "/uploads/a.jpg", filename: "a.jpg" }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ path: "/uploads/a.jpg", filename: "a.jpg" }),
      });

    renderTerminal();

    const input = document.querySelector('input[type="file"][accept="image/*"]') as HTMLInputElement;
    const file = new File(["x"], "photo.jpg", { type: "image/jpeg" });
    Object.defineProperty(input, "files", { value: [file], configurable: true });

    await act(async () => {
      fireEvent.change(input);
    });

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledTimes(1);
    });

    // Simulate second selection (input value would have been reset by handler)
    await act(async () => {
      fireEvent.change(input);
    });

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledTimes(2);
    });
  });
});
