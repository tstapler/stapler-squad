import { act, renderHook } from "@testing-library/react";
import { Code, ConnectError } from "@connectrpc/connect";
import { ReadOnlyReason, ReadOnlyReasonDetailSchema } from "@/gen/session/v1/session_pb";
import { createFilesTestHarness } from "@/lib/files/__tests__/fixtures";
import { controllableSearchStream, searchQuery } from "@/lib/files/__tests__/fixtures/controllableSearchStream";
import { useContentSearch } from "../useContentSearch";

let onFlagOff: () => Promise<void> = async () => undefined;
jest.mock("../useFilesFlags", () => ({ useFilesFlags: () => ({ onFlagOff }) }));
const latest = jest.fn(async () => undefined);
const stale = jest.fn(async () => undefined);

describe("useContentSearch FlagOff refresh", () => {
  it("useContentSearch_should_CallTheLatestOnFlagOff_When_RefusedAfterTheFlagsHookChanged", async () => {
    const h = createFilesTestHarness();
    const c = controllableSearchStream();
    onFlagOff = stale;
    const view = renderHook(() => useContentSearch({ sessionId: "s1", stream: c.stream }), { wrapper: h.wrapper });
    act(() => view.result.current.runNow(searchQuery("foo")));
    onFlagOff = latest; // flags context re-created its refresh after the search started
    view.rerender();

    await act(async () => {
      c.requests[0].fail(new ConnectError("viewer v2 disabled", Code.FailedPrecondition, undefined, [
        { desc: ReadOnlyReasonDetailSchema, value: { reason: ReadOnlyReason.FLAG_OFF } },
      ]));
    });

    expect(latest).toHaveBeenCalledTimes(1);
    expect(stale).not.toHaveBeenCalled();
  });
});
