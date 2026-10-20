import { act, renderHook, waitFor } from "@testing-library/react";
import { ConnectError, Code } from "@connectrpc/connect";
import { createFilesTestHarness } from "@/lib/files/__tests__/fixtures";
import { useFilesFlags } from "../useFilesFlags";

type Flags = Record<string, boolean>;

function setup(initial: Flags, fail = false) {
  const server = { flags: initial, fail };
  const harness = createFilesTestHarness({
    handlers: {
      getFeatureFlags: () => {
        if (server.fail) throw new ConnectError("down", Code.Unavailable);
        return { flags: Object.entries(server.flags).map(([name, enabled]) => ({ name, enabled })) };
      },
    },
  });
  const hook = renderHook(() => useFilesFlags(), { wrapper: harness.wrapper });
  return { server, harness, ...hook };
}

describe("useFilesFlags", () => {
  afterEach(() => jest.useRealTimers());

  it("useFilesFlags_should_ReturnReadyWithServerValues_When_GetFeatureFlagsSucceeds", async () => {
    const { result } = setup({ "files:editing": true, "files:viewer-v2": true, "files:range-read": true });
    await waitFor(() => expect(result.current.status).toBe("ready"));
    expect(result.current).toMatchObject({
      status: "ready", editing: true, viewerV2: true, rangeRead: true, deltaWrite: false, editLarge: false,
    });
  });

  it("useFilesFlags_should_ReturnLoadingNotOff_And_MountNeitherLayout_When_FlagsPending", async () => {
    const { result } = setup({});
    expect(result.current.status).toBe("loading");
    await waitFor(() => expect(result.current.status).toBe("ready"));
  });

  it("useFilesFlags_should_UseServerDefaults_When_FlagsAbsentFromTheResponse", async () => {
    const { result } = setup({});
    await waitFor(() => expect(result.current.status).toBe("ready"));
    expect(result.current).toMatchObject({
      editing: false, viewerV2: true, mergePhone: true, rangeRead: false, deltaWrite: false, editLarge: false,
    });
  });

  it("useFilesFlags_should_FailClosedEditingFalse_When_GetFeatureFlagsErrors", async () => {
    const { result } = setup({ "files:editing": true, "files:delta-write": true }, true);
    await waitFor(() => expect(result.current.status).toBe("error"));
    expect(result.current).toMatchObject({
      editing: false, deltaWrite: false, rangeRead: false, editLarge: false, viewerV2: true,
    });
  });

  it("featureFlags_should_RefreshOnFocusAndOnFlagOffRefusal_And_DropEditorToReadOnly", async () => {
    jest.useFakeTimers({ doNotFake: ["queueMicrotask", "nextTick"] });
    const { result, server, harness } = setup({ "files:editing": true });
    await waitFor(() => expect(result.current.editing).toBe(true));
    const fetches = () => harness.calls.filter((c) => c === "getFeatureFlags").length;
    const initial = fetches();

    server.flags = { "files:editing": false };
    act(() => void jest.advanceTimersByTime(6000));
    await act(async () => void window.dispatchEvent(new Event("focus")));
    await waitFor(() => expect(result.current.editing).toBe(false));
    expect(fetches()).toBe(initial + 1);

    // Throttled: a second focus within 5 s does not re-fetch.
    await act(async () => void window.dispatchEvent(new Event("focus")));
    expect(fetches()).toBe(initial + 1);

    // A FlagOff refusal bypasses the throttle.
    server.flags = { "files:editing": true };
    await act(async () => void (await result.current.onFlagOff()));
    await waitFor(() => expect(result.current.editing).toBe(true));
    expect(fetches()).toBe(initial + 2);
  });

  it("featureFlags_should_NotChangeViewerV2_When_FlippedMidSession", async () => {
    const { result, server } = setup({ "files:viewer-v2": true });
    await waitFor(() => expect(result.current.status).toBe("ready"));
    server.flags = { "files:viewer-v2": false };
    await act(async () => void (await result.current.onFlagOff()));
    expect(result.current.viewerV2).toBe(true);
  });
});
