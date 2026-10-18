import { renderHook, act, waitFor } from "@testing-library/react";

const getFileContent = jest.fn();
jest.mock("@connectrpc/connect", () => ({ createClient: () => ({ getFileContent }) }));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));

import { useGetFileContent } from "./useFileService";

const reply = (content: string) => ({ content, isBinary: false, isTruncated: false });

describe("useGetFileContent", () => {
  beforeEach(() => getFileContent.mockReset());

  it("useGetFileContent_should_NotReturnPreviousFileContent_When_PathChangesToACachedFile", async () => {
    getFileContent.mockImplementation(async (req: { path: string }) => reply(`content of ${req.path}`));
    const { result, rerender } = renderHook(({ path }) => useGetFileContent("s-cache", path, ""), { initialProps: { path: "a.txt" } });
    await waitFor(() => expect(result.current.data?.content).toBe("content of a.txt"));
    rerender({ path: "b.txt" });
    await waitFor(() => expect(result.current.data?.content).toBe("content of b.txt"));

    const seen: Array<string | undefined> = [];
    const probe = renderHook(({ path }) => { const r = useGetFileContent("s-cache", path, ""); seen.push(r.data?.content); return r; }, { initialProps: { path: "a.txt" } });
    await waitFor(() => expect(probe.result.current.data?.content).toBe("content of a.txt"));
    seen.length = 0;
    await act(async () => probe.rerender({ path: "b.txt" })); // b.txt is now served from the module cache
    expect(seen).not.toContain("content of a.txt");
    expect(probe.result.current.data?.content).toBe("content of b.txt");
  });
});
