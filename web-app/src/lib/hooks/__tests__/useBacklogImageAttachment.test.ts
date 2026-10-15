import { renderHook, act } from "@testing-library/react";
import { useBacklogImageAttachment } from "@/lib/hooks/useBacklogImageAttachment";

jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "" }));

let fetchSpy: jest.SpyInstance;

beforeEach(() => {
  if (!global.fetch) global.fetch = jest.fn() as unknown as typeof fetch;
  fetchSpy = jest.spyOn(global, "fetch");
});

afterEach(() => {
  fetchSpy.mockRestore();
});

function png(name = "a.png") {
  return new File(["x"], name, { type: "image/png" });
}

function okResponse(filename: string) {
  return { ok: true, status: 200, json: async () => ({ path: `/tmp/${filename}`, filename }) } as Response;
}

function setup(initial = "hello world") {
  const textarea = document.createElement("textarea");
  textarea.value = initial;
  document.body.appendChild(textarea);
  let value = initial;
  const setValue = jest.fn((updater: (prev: string) => string) => {
    value = updater(value);
    textarea.value = value;
  });
  const hook = renderHook(() => useBacklogImageAttachment({ textareaRef: { current: textarea }, setValue }));
  return { ...hook, textarea, getValue: () => value };
}

describe("useBacklogImageAttachment", () => {
  it("inserts markdown at the cursor on success", async () => {
    fetchSpy.mockResolvedValue(okResponse("a.png"));
    const { result, textarea, getValue } = setup("hello world");
    textarea.setSelectionRange(5, 5);

    await act(async () => {
      await result.current.onFileInputChange({
        target: { files: [png()], value: "x" },
      } as never);
    });

    expect(getValue()).toBe("hello![a.png](/api/local/serve/tmp/a.png) world");
    expect(result.current.error).toBeNull();
    expect(result.current.uploading).toBe(false);
  });

  it("rejects non-image files without uploading", async () => {
    const { result } = setup();
    await act(async () => {
      result.current.onFileInputChange({
        target: { files: [new File(["x"], "a.txt", { type: "text/plain" })], value: "x" },
      } as never);
    });
    expect(result.current.error).toBe("Only image files can be attached.");
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it.each([
    [413, "Image is too large (max 10 MB)."],
    [415, "Unsupported image type — use PNG, JPEG, GIF, or WebP."],
    [500, "Upload failed."],
  ])("maps HTTP %i to its message", async (status, message) => {
    fetchSpy.mockResolvedValue({ ok: false, status } as Response);
    const { result, getValue } = setup();
    await act(async () => {
      result.current.onFileInputChange({ target: { files: [png()], value: "x" } } as never);
    });
    expect(result.current.error).toBe(message);
    expect(getValue()).toBe("hello world");
    expect(result.current.uploading).toBe(false);
  });

  it("reports a network error", async () => {
    fetchSpy.mockRejectedValue(new Error("down"));
    const { result } = setup();
    await act(async () => {
      result.current.onFileInputChange({ target: { files: [png()], value: "x" } } as never);
    });
    expect(result.current.error).toBe("Network error — upload failed.");
    expect(result.current.uploading).toBe(false);
  });

  it("keeps uploading true until both concurrent uploads finish", async () => {
    const resolvers: Array<(r: Response) => void> = [];
    fetchSpy.mockImplementation(() => new Promise<Response>((resolve) => resolvers.push(resolve)));
    const { result } = setup();

    act(() => {
      result.current.onFileInputChange({ target: { files: [png("1.png")], value: "x" } } as never);
      result.current.onFileInputChange({ target: { files: [png("2.png")], value: "x" } } as never);
    });
    expect(result.current.uploading).toBe(true);

    await act(async () => {
      resolvers[0](okResponse("1.png"));
    });
    expect(result.current.uploading).toBe(true);

    await act(async () => {
      resolvers[1](okResponse("2.png"));
    });
    expect(result.current.uploading).toBe(false);
  });

  it("does not update state or insert after unmount", async () => {
    let resolve!: (r: Response) => void;
    fetchSpy.mockImplementation(() => new Promise<Response>((r) => { resolve = r; }));
    const { result, unmount, getValue } = setup();
    act(() => {
      result.current.onFileInputChange({ target: { files: [png()], value: "x" } } as never);
    });
    unmount();
    await act(async () => {
      resolve(okResponse("a.png"));
    });
    expect(getValue()).toBe("hello world");
  });
});
