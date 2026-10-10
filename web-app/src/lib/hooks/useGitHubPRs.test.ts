import { renderHook, act, waitFor } from "@testing-library/react";
import { createClient } from "@connectrpc/connect";
import { create, type MessageInitShape } from "@bufbuild/protobuf";
import {
  AccountPollState,
  AccountPollStatusSchema,
  UserPREventSchema,
  type UserPREvent,
} from "@/gen/session/v1/github_user_pb";
import { UserPRSchema } from "@/gen/session/v1/types_pb";
import { useGitHubPRs } from "./useGitHubPRs";

jest.mock("@connectrpc/connect");
jest.mock("@/lib/api/transport", () => ({ getWatchTransport: () => ({}) }));

/** A watch stream the test pushes events into or fails on demand. */
class ControlledStream {
  private queue: Array<{ event?: UserPREvent; error?: Error }> = [];
  private waiter: (() => void) | null = null;
  push(event: UserPREvent) {
    this.queue.push({ event });
    this.waiter?.();
  }
  fail(error: Error) {
    this.queue.push({ error });
    this.waiter?.();
  }
  async *[Symbol.asyncIterator]() {
    for (;;) {
      while (this.queue.length === 0) await new Promise<void>((r) => (this.waiter = r));
      const next = this.queue.shift()!;
      if (next.error) throw next.error;
      yield next.event!;
    }
  }
}

let streams: ControlledStream[];

beforeEach(() => {
  streams = [];
  (createClient as jest.Mock).mockReturnValue({
    watchUserPRs: () => {
      const s = new ControlledStream();
      streams.push(s);
      return s;
    },
  });
});

const pr = (n: number) => create(UserPRSchema, { owner: "acme", repo: "api", number: n });
const event = (over: MessageInitShape<typeof UserPREventSchema>) =>
  create(UserPREventSchema, { eventType: "snapshot", ...over });

describe("useGitHubPRs", () => {
  it("useGitHubPRs_should_KeepLastSnapshotAndExposeErrorAndLastUpdatedAt_When_RefreshFails", async () => {
    let clock = 1_000;
    const { result } = renderHook(() => useGitHubPRs({ now: () => clock }));
    await waitFor(() => expect(streams).toHaveLength(1));

    act(() => streams[0].push(event({ prs: [pr(1), pr(2)] })));
    await waitFor(() => expect(result.current.prs).toHaveLength(2));
    expect(result.current.lastUpdatedAt).toBe(1_000);
    expect(result.current.error).toBeUndefined();

    clock = 5_000;
    act(() => result.current.refresh());
    expect(result.current.refreshing).toBe(true);
    await waitFor(() => expect(streams).toHaveLength(2));
    act(() => streams[1].fail(new Error("boom")));

    await waitFor(() => expect(result.current.error).toBe("boom"));
    expect(result.current.refreshing).toBe(false);
    expect(result.current.prs).toHaveLength(2);
    expect(result.current.lastUpdatedAt).toBe(1_000);

    // recovery clears the error and advances the timestamp
    act(() => result.current.refresh());
    await waitFor(() => expect(streams).toHaveLength(3));
    clock = 9_000;
    act(() => streams[2].push(event({ prs: [pr(3)] })));
    await waitFor(() => expect(result.current.error).toBeUndefined());
    expect(result.current.prs).toHaveLength(1);
    expect(result.current.lastUpdatedAt).toBe(9_000);
  });

  it("useGitHubPRs_should_ExposeAccountStatusesAndKeepOtherAccountsPRs_When_OneAccountUnauthorized", async () => {
    const { result } = renderHook(() => useGitHubPRs());
    await waitFor(() => expect(streams).toHaveLength(1));

    const statuses = [
      create(AccountPollStatusSchema, { host: "ghe.corp", accountLogin: "bob", state: AccountPollState.UNAUTHORIZED }),
      create(AccountPollStatusSchema, { host: "github.com", accountLogin: "alice", state: AccountPollState.OK }),
    ];
    act(() => streams[0].push(event({ prs: [pr(1)], accountStatuses: statuses })));

    await waitFor(() => expect(result.current.accountStatuses).toHaveLength(2));
    expect(result.current.accountStatuses[0].state).toBe(AccountPollState.UNAUTHORIZED);
    expect(result.current.accountStatuses[1].accountLogin).toBe("alice");
    expect(result.current.prs.map((p) => p.number)).toEqual([1]);

    // a delta event without statuses must not wipe the last known statuses
    act(() => streams[0].push(event({ eventType: "updated", prs: [pr(1)] })));
    await waitFor(() => expect(result.current.prs).toHaveLength(1));
    expect(result.current.accountStatuses).toHaveLength(2);
  });
});
