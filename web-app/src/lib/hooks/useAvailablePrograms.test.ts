import { renderHook, waitFor } from "@testing-library/react";
import { PROGRAMS, type ProgramOption } from "@/lib/constants/programs";

// jest.setup.js globally mocks this hook; these tests need the real one.
const { useAvailablePrograms, resetAvailableProgramsForTest } = jest.requireActual<{
  useAvailablePrograms: () => ProgramOption[];
  resetAvailableProgramsForTest: () => void;
}>("./useAvailablePrograms");

const mockList = jest.fn();
jest.mock("@connectrpc/connect", () => ({ createClient: () => ({ listProgramsConfig: mockList }) }));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));

describe("useAvailablePrograms", () => {
  beforeAll(() => {
    // jsdom has no fetch; the hook bails out early without one.
    global.fetch = jest.fn() as unknown as typeof fetch;
  });

  beforeEach(() => {
    resetAvailableProgramsForTest();
    mockList.mockReset();
    (global.fetch as jest.Mock).mockReset();
  });

  it("useAvailablePrograms_should_IssueOneListProgramsConfig_When_58ConsumersMount", async () => {
    mockList.mockResolvedValue({ programs: [{ id: "aider", label: "Aider", description: "", command: "aider", cliFlags: "" }] });
    const hooks = Array.from({ length: 58 }, () => renderHook(() => useAvailablePrograms()));
    await waitFor(() => hooks.forEach((h) => expect(h.result.current[0].value).toBe("aider")));
    expect(mockList).toHaveBeenCalledTimes(1);
    const late = renderHook(() => useAvailablePrograms());
    await waitFor(() => expect(late.result.current[0].value).toBe("aider"));
    expect(mockList).toHaveBeenCalledTimes(1);
  });

  it("useAvailablePrograms_should_IssueOneServerInfoFallback_And_NotCacheFailure_When_RpcRejectsAnd10ConsumersMount", async () => {
    mockList.mockRejectedValue(new Error("down"));
    (global.fetch as jest.Mock).mockResolvedValue({ json: () => Promise.resolve({ programs: ["/usr/bin/extra-cli"] }) });
    const hooks = Array.from({ length: 10 }, () => renderHook(() => useAvailablePrograms()));
    await waitFor(() => hooks.forEach((h) => expect(h.result.current.at(-1)?.value).toBe("extra-cli")));
    expect(global.fetch).toHaveBeenCalledTimes(1);
    expect(mockList).toHaveBeenCalledTimes(1);

    // Failure is not cached: a later mount retries the RPC.
    mockList.mockResolvedValue({ programs: [{ id: "aider", label: "Aider", description: "", command: "aider", cliFlags: "" }] });
    const retry = renderHook(() => useAvailablePrograms());
    await waitFor(() => expect(retry.result.current[0].value).toBe("aider"));
    expect(mockList).toHaveBeenCalledTimes(2);
  });

  it("useAvailablePrograms_should_CarryCommandAndCliFlags_When_ListProgramsConfigMapped", async () => {
    mockList.mockResolvedValue({
      programs: [{ id: "aider", label: "Aider", description: "", command: "aider", cliFlags: "--yes-always" }],
    });
    const { result } = renderHook(() => useAvailablePrograms());
    await waitFor(() => expect(result.current[0].value).toBe("aider"));
    expect(result.current[0]).toMatchObject({ value: "aider", command: "aider", cliFlags: "--yes-always" });
  });

  it("useAvailablePrograms_should_KeepStaticProgramsWorking_When_CommandFieldsAbsent", () => {
    mockList.mockReturnValue(new Promise(() => {}));
    const { result } = renderHook(() => useAvailablePrograms());
    expect(result.current).toBe(PROGRAMS);
    expect(result.current.every((p) => p.command === undefined && p.cliFlags === undefined)).toBe(true);
  });

  describe("server-info fallback abort", () => {
    function hangingFetch(): AbortSignal[] {
      const signals: AbortSignal[] = [];
      (global.fetch as jest.Mock).mockImplementation((_url: string, init: { signal: AbortSignal }) => {
        signals.push(init.signal);
        return new Promise(() => {});
      });
      return signals;
    }

    it("useAvailablePrograms_should_AbortServerInfoFallback_When_LastSubscriberUnmounts", async () => {
      mockList.mockRejectedValue(new Error("down"));
      const signals = hangingFetch();
      const a = renderHook(() => useAvailablePrograms());
      const b = renderHook(() => useAvailablePrograms());
      await waitFor(() => expect(signals).toHaveLength(1));

      a.unmount();
      expect(signals[0].aborted).toBe(false); // another consumer still waits on the shared flight
      b.unmount();
      expect(signals[0].aborted).toBe(true);
    });

    it("useAvailablePrograms_should_StartFreshFlight_When_RemountedAfterAbort", async () => {
      mockList.mockRejectedValue(new Error("down"));
      const signals = hangingFetch();
      const first = renderHook(() => useAvailablePrograms());
      await waitFor(() => expect(signals).toHaveLength(1));
      first.unmount();
      renderHook(() => useAvailablePrograms());
      await waitFor(() => expect(signals).toHaveLength(2));
      expect(signals[1].aborted).toBe(false);
    });
  });
});
