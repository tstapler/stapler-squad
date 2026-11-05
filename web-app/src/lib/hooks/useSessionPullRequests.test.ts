import { act, renderHook } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { SessionSchema } from "@/gen/session/v1/types_pb";
import { MergeMethod } from "@/gen/session/v1/session_pb";
import { makePr } from "@/lib/pr/__testUtils__/pullRequests";
import { useSessionPullRequests } from "./useSessionPullRequests";

const client = {
  refreshSessionPullRequests: jest.fn(), mergePR: jest.fn(), closePR: jest.fn(),
  pinSessionPullRequest: jest.fn(), detachSessionPullRequest: jest.fn(), attachSessionPullRequest: jest.fn(),
};
jest.mock("@connectrpc/connect", () => ({ ...jest.requireActual("@connectrpc/connect"), createClient: () => client }));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));

const session = () => create(SessionSchema, { id: "s1", pullRequests: [makePr({ number: 12, isActive: true })] });
// A session object keeps its identity until the server pushes a new one; a fresh one per render would reset the list.
const stable = session();
const target = { host: "github.com", owner: "acme", repo: "app", number: 14 };

beforeEach(() => Object.values(client).forEach((f) => f.mockReset()));

describe("useSessionPullRequests", () => {
  it("merge_should_sendTheRowRefAndMethodThenRefreshTheList", async () => {
    client.mergePR.mockResolvedValue({ success: true, message: "" });
    client.refreshSessionPullRequests.mockResolvedValue({ pullRequests: [makePr({ number: 12, isActive: true }), makePr({ number: 14, state: "merged" })] });
    const { result } = renderHook(() => useSessionPullRequests(stable));
    let outcome;
    await act(async () => { outcome = await result.current.actions.merge(target, "rebase"); });
    expect(outcome).toEqual({ ok: true });
    expect(client.mergePR).toHaveBeenCalledWith(expect.objectContaining({ id: "s1", mergeMethod: MergeMethod.REBASE, pr: expect.objectContaining({ number: 14, repo: "app" }) }));
    expect(result.current.pullRequests.map((p) => p.number)).toEqual([12, 14]);
  });

  it("merge_should_returnTheFailureReason_When_ResponseIsNotSuccessful", async () => {
    client.mergePR.mockResolvedValue({ success: false, message: "Pull Request is already merged" });
    const { result } = renderHook(() => useSessionPullRequests(stable));
    let outcome;
    await act(async () => { outcome = await result.current.actions.merge(target, "squash"); });
    expect(outcome).toMatchObject({ ok: false, kind: "already_merged" });
    expect(client.refreshSessionPullRequests).not.toHaveBeenCalled();
  });

  it("detach_should_replaceTheListFromTheResponseAndSurfaceErrors", async () => {
    client.detachSessionPullRequest.mockResolvedValueOnce({ pullRequests: [] });
    const { result } = renderHook(() => useSessionPullRequests(stable));
    await act(async () => { await result.current.actions.detach(target); });
    expect(result.current.pullRequests).toEqual([]);
    expect(client.detachSessionPullRequest).toHaveBeenCalledWith(expect.objectContaining({ id: "s1", pr: expect.objectContaining({ number: 14 }) }));
    client.detachSessionPullRequest.mockRejectedValueOnce(new ConnectError("x is not attached to this session", Code.FailedPrecondition));
    let outcome;
    await act(async () => { outcome = await result.current.actions.detach(target); });
    expect(outcome).toMatchObject({ ok: false, kind: "not_attached" });
  });

  it("pin_should_clearThePin_When_TargetIsNull_AndAttachShouldSendTheUrl", async () => {
    client.pinSessionPullRequest.mockResolvedValue({ pullRequests: [] });
    client.attachSessionPullRequest.mockResolvedValue({ pullRequests: [makePr({ number: 5 })] });
    const { result } = renderHook(() => useSessionPullRequests(stable));
    await act(async () => { await result.current.actions.pin(null); });
    expect(client.pinSessionPullRequest).toHaveBeenCalledWith({ id: "s1", pr: undefined });
    await act(async () => { await result.current.actions.attachByUrl("https://github.com/acme/app/pull/5"); });
    expect(client.attachSessionPullRequest).toHaveBeenCalledWith({ id: "s1", url: "https://github.com/acme/app/pull/5" });
    expect(result.current.pullRequests.map((p) => p.number)).toEqual([5]);
  });

  it("list_should_followTheSessionAgain_When_ANewListIsPushed", async () => {
    client.pinSessionPullRequest.mockResolvedValue({ pullRequests: [makePr({ number: 99 })] });
    const { result, rerender } = renderHook(({ s }) => useSessionPullRequests(s), { initialProps: { s: session() } });
    await act(async () => { await result.current.actions.pin(target); });
    expect(result.current.pullRequests[0].number).toBe(99);
    rerender({ s: session() });
    expect(result.current.pullRequests[0].number).toBe(12);
  });
});
