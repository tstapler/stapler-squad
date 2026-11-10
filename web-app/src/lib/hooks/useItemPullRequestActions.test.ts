import { renderHook } from "@testing-library/react";
import { Code, ConnectError } from "@connectrpc/connect";
import { useItemPullRequestActions } from "./useItemPullRequestActions";

const detachItemPullRequest = jest.fn();
jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({ detachItemPullRequest: (...a: unknown[]) => detachItemPullRequest(...a) }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@/lib/hooks/useGitHubEnterpriseHosts", () => ({ useGitHubEnterpriseHosts: () => ({ hosts: [] }) }));

const setup = (associate = jest.fn().mockResolvedValue(undefined), onChanged = jest.fn()) =>
  ({ associate, onChanged, ...renderHook(() => useItemPullRequestActions({ itemId: "item-1", associate, onChanged })).result.current });

describe("useItemPullRequestActions", () => {
  beforeEach(() => detachItemPullRequest.mockReset().mockResolvedValue({}));

  it("stopTracking_should_sendTheItemIdAndTheRowsRef_ThenReload", async () => {
    const { stopTracking, onChanged } = setup();
    const result = await stopTracking({ host: "github.com", owner: "acme", repo: "app", number: 23 });
    expect(result.ok).toBe(true);
    expect(detachItemPullRequest).toHaveBeenCalledWith({ itemId: "item-1", pr: expect.objectContaining({ owner: "acme", repo: "app", number: 23, host: "github.com" }) });
    expect(onChanged).toHaveBeenCalledTimes(1);
  });

  it("stopTracking_should_returnTheFailureAndNotReload_When_TheRpcRejects", async () => {
    detachItemPullRequest.mockRejectedValueOnce(new Error("boom"));
    const { stopTracking, onChanged } = setup();
    const result = await stopTracking({ host: "", owner: "a", repo: "b", number: 1 });
    expect(result).toMatchObject({ ok: false, message: expect.stringContaining("boom") });
    expect(onChanged).not.toHaveBeenCalled();
  });

  it("trackByUrl_should_rejectANonPrUrlWithoutCallingTheServer", async () => {
    const { trackByUrl, associate } = setup();
    expect(await trackByUrl("https://example.com/x")).toMatchObject({ ok: false, kind: "invalid_url" });
    expect(associate).not.toHaveBeenCalled();
  });

  it("trackByUrl_should_passTheUrlAndParsedNumber_AndSurfaceTheServersReason", async () => {
    const ok = setup();
    expect((await ok.trackByUrl("https://github.com/acme/app/pull/24")).ok).toBe(true);
    expect(ok.associate).toHaveBeenCalledWith("https://github.com/acme/app/pull/24", 24);

    const bad = setup(jest.fn().mockRejectedValue(new Error("item is not in review")));
    expect(await bad.trackByUrl("https://github.com/acme/app/pull/24")).toMatchObject({ ok: false, message: expect.stringContaining("not in review") });
  });

  it("trackByUrl_should_showTheServersFailedPreconditionMessageVerbatim", async () => {
    const message = "item is in status ready; PR can only be linked in review or pr_pending";
    const bad = setup(jest.fn().mockRejectedValue(new ConnectError(message, Code.FailedPrecondition)));
    expect(await bad.trackByUrl("https://github.com/acme/app/pull/24")).toEqual({ ok: false, kind: "other", message });
  });
});
