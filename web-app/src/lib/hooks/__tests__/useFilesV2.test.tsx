import { act, renderHook, waitFor } from "@testing-library/react";
import { createFakeTransport, createFilesTestHarness } from "@/lib/files/__tests__/fixtures";
import { Rejected, type FileStat, type FileTransportPort } from "@/lib/files/transport/fileTransport";
import { useFilesV2 } from "../useFilesV2";

jest.mock("../useFileService", () => ({ invalidateFileContent: jest.fn() }));

const FILES = { "a.go": "x", "b.go": "y" };
const STAT: FileStat = { token: "v1:1:9:fresh", size: 1, readOnly: "none", analysis: "unspecified" };

/** `a.go` stats hang once armed, so a reload can be left in flight while the user switches tabs. */
function setup() {
  const base = createFakeTransport({ files: FILES });
  let armed = false;
  let settle: { resolve: (s: FileStat) => void; reject: (e: unknown) => void } | null = null;
  const stat: FileTransportPort["stat"] = (ref, o, signal) => {
    if (!armed || ref.path !== "a.go") return base.stat(ref, o, signal);
    return new Promise<FileStat>((resolve, reject) => { settle = { resolve, reject }; });
  };
  const h = createFilesTestHarness({ sessionId: "s1", transport: { ...base, stat } });
  const view = renderHook(() => useFilesV2("s1", false, {}), { wrapper: h.wrapper });
  const loaded = (path: string) => waitFor(() => expect(view.result.current.buffers.active).toMatchObject({ path, status: "loaded" }));
  return { ...view, loaded, arm: () => { armed = true; }, settle: () => settle! };
}

describe("useFilesV2 reloadFromDisk", () => {
  beforeEach(() => window.localStorage.clear());

  it("reloadFromDisk_should_NotYankBackToTheOldTab_When_UserSwitchedBeforeStatReturned", async () => {
    const { result, loaded, arm, settle } = setup();
    act(() => result.current.buffers.open("a.go", { pinned: true }));
    await loaded("a.go");
    arm();
    act(() => result.current.reloadFromDisk());
    act(() => result.current.buffers.open("b.go", { pinned: true }));
    await loaded("b.go");

    await act(async () => settle().resolve(STAT));

    expect(result.current.buffers.active?.path).toBe("b.go");
    expect(result.current.feedback.message?.kind).not.toBe("success");
  });

  it("reloadFromDisk_should_StayQuiet_When_StatFailsAfterTheUserSwitchedTabs", async () => {
    const { result, loaded, arm, settle } = setup();
    act(() => result.current.buffers.open("a.go", { pinned: true }));
    await loaded("a.go");
    arm();
    act(() => result.current.reloadFromDisk());
    act(() => result.current.buffers.open("b.go", { pinned: true }));
    await loaded("b.go");

    await act(async () => settle().reject(new Rejected("not_found")));

    expect(result.current.reloadFailed).toBeNull();
    expect(result.current.feedback.message?.kind).not.toBe("error");
  });

  it("reloadFromDisk_should_NameTheRealFailure_When_StatSaysNotFound", async () => {
    const { result, loaded, arm, settle } = setup();
    act(() => result.current.buffers.open("a.go", { pinned: true }));
    await loaded("a.go");
    arm();
    act(() => result.current.reloadFromDisk());

    await act(async () => settle().reject(new Rejected("not_found")));

    expect(result.current.reloadFailed?.path).toBe("a.go");
    expect(result.current.feedback.message?.text).toContain("not found");
    expect(result.current.feedback.message?.text).not.toContain("network error");
  });
});
