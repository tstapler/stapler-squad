import { act, renderHook } from "@testing-library/react";
import { Code, ConnectError } from "@connectrpc/connect";
import { ProbeStatus } from "@/gen/session/v1/session_pb";
import { useProbeProgram } from "./useProbeProgram";
import {
  deferred,
  mockProbeClient,
  probeResponse,
  resetProbeMocks,
} from "@/lib/hooks/__mocks__/probeProgramMock";

jest.mock("@connectrpc/connect", () =>
  require("@/lib/hooks/__mocks__/probeProgramMock").connectMockFactory(),
);
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));

const probe = mockProbeClient.probeProgram;

beforeEach(() => {
  resetProbeMocks();
  probe.mockResolvedValue(probeResponse());
});

describe("useProbeProgram", () => {
  it("useProbeProgram_should_SetFoundState_When_CheckResolvesFound", async () => {
    probe.mockResolvedValue(
      probeResponse({ probeStatus: ProbeStatus.FOUND_PARSED, flags: [{}, {}] }),
    );
    const { result } = renderHook(() => useProbeProgram("claude --x", "program-config"));
    expect(result.current.state.kind).toBe("idle");
    await act(async () => result.current.check());
    expect(result.current.state).toEqual({
      kind: "found",
      path: "/usr/bin/claude",
      flagCount: 2,
      flags: [{}, {}],
    });
    expect(result.current.checkedToken).toBe("claude");
    // blur sends neither flag
    expect(probe.mock.calls[0][0]).toEqual({
      command: "claude --x",
      confirmExecute: false,
      resolveOnly: false,
    });
  });

  it("useProbeProgram_should_IgnoreStaleResponse_When_FirstProbeResolvesLast", async () => {
    const first = deferred<unknown>();
    const second = deferred<unknown>();
    probe.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { result, rerender } = renderHook(
      ({ cmd }) => useProbeProgram(cmd, "program-config"),
      { initialProps: { cmd: "clau" } },
    );
    act(() => result.current.check());
    rerender({ cmd: "claude" });
    act(() => result.current.check());
    await act(async () => {
      second.resolve(probeResponse({ resolvedPath: "/bin/claude" }));
      await second.promise;
    });
    await act(async () => {
      first.resolve(probeResponse({ resolvedPath: "/bin/clau" }));
      await first.promise;
    });
    expect(result.current.state).toMatchObject({ kind: "found", path: "/bin/claude" });
  });

  it("useProbeProgram_should_MapToTransportError_When_RpcRejectsOrPermissionDenied", async () => {
    const { result } = renderHook(() => useProbeProgram("claude", "program-config"));
    for (const err of [
      new ConnectError("guard", Code.PermissionDenied),
      Object.assign(new Error("Forbidden"), { status: 403 }),
      new Error("network down"),
    ]) {
      probe.mockRejectedValueOnce(err);
      await act(async () => result.current.check({ explicit: true }));
      expect(result.current.state.kind).toBe("transportError");
    }
  });

  it("useProbeProgram_should_MapUnimplementedToDisabledAndRenderNothing_When_KillSwitchOff", async () => {
    probe.mockRejectedValue(new ConnectError("off", Code.Unimplemented));
    const { result } = renderHook(() => useProbeProgram("claude", "program-config"));
    await act(async () => result.current.check());
    expect(result.current.state.kind).toBe("disabled");
  });

  it("useProbeProgram_should_MapToBusyOrError_When_StatusErrorOrBusy", async () => {
    const { result } = renderHook(() => useProbeProgram("claude", "program-config"));
    for (const status of [ProbeStatus.ERROR, ProbeStatus.BUSY]) {
      probe.mockResolvedValueOnce(probeResponse({ found: false, probeStatus: status }));
      await act(async () => result.current.check({ explicit: true }));
      expect(result.current.state.kind).toBe("busyOrError");
    }
  });

  it("useProbeProgram_should_MapStatusesAndSendRequestShape_When_ExplicitOrPicker", async () => {
    const { result } = renderHook(() => useProbeProgram("./run.sh", "picker"));
    probe.mockResolvedValueOnce(probeResponse({ probeStatus: ProbeStatus.NEEDS_CONFIRM }));
    await act(async () => result.current.check());
    expect(probe.mock.calls[0][0]).toMatchObject({ confirmExecute: false, resolveOnly: true });
    expect(result.current.state).toEqual({ kind: "needsConfirm", path: "/usr/bin/claude" });

    probe.mockResolvedValueOnce(probeResponse({ probeStatus: ProbeStatus.FOUND_PARSED }));
    await act(async () => result.current.check({ explicit: true }));
    expect(probe.mock.calls[1][0]).toMatchObject({ confirmExecute: true, resolveOnly: false });

    probe.mockResolvedValueOnce(probeResponse({ probeStatus: ProbeStatus.TIMEOUT }));
    await act(async () => result.current.check({ explicit: true }));
    expect(result.current.state.kind).toBe("timeout");

    probe.mockResolvedValueOnce(probeResponse({ isWrapper: true }));
    await act(async () => result.current.check({ immediate: true }));
    expect(result.current.state.kind).toBe("wrapper");

    probe.mockResolvedValueOnce(probeResponse({ found: false, probeStatus: ProbeStatus.NOT_FOUND }));
    await act(async () => result.current.check({ immediate: true }));
    expect(result.current.state.kind).toBe("notFound");
  });

  it("useProbeProgram_should_AbortAndSkip_When_UnmountOrEmptyCommand", async () => {
    const pending = deferred<unknown>();
    probe.mockReturnValue(pending.promise);
    const empty = renderHook(() => useProbeProgram("   ", "program-config"));
    act(() => empty.result.current.check());
    expect(probe).not.toHaveBeenCalled();
    expect(empty.result.current.state.kind).toBe("idle");

    const { result, unmount } = renderHook(() => useProbeProgram("claude", "program-config"));
    act(() => result.current.check());
    act(() => result.current.check()); // double-submit guard
    expect(probe).toHaveBeenCalledTimes(1);
    const signal = probe.mock.calls[0][1].signal as AbortSignal;
    expect(result.current.state.kind).toBe("checking");
    unmount();
    expect(signal.aborted).toBe(true);
  });

  it("useProbeProgram_should_MemoizeFoundButNotErrors_When_SameCommandRechecked", async () => {
    const { result } = renderHook(() => useProbeProgram("claude", "program-config"));
    await act(async () => result.current.check());
    await act(async () => result.current.check());
    expect(probe).toHaveBeenCalledTimes(1);

    const errored = renderHook(() => useProbeProgram("other", "program-config"));
    probe.mockRejectedValueOnce(new Error("boom"));
    await act(async () => errored.result.current.check());
    await act(async () => errored.result.current.check());
    expect(probe).toHaveBeenCalledTimes(3);
    expect(errored.result.current.state.kind).toBe("found");
  });
});
