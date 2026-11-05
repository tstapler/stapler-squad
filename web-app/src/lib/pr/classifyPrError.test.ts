import { Code, ConnectError } from "@connectrpc/connect";
import { classifyPrError, classifyPrMessage } from "./classifyPrError";

describe("classifyPrError", () => {
  it.each([
    [new ConnectError("123 is not attached to this session", Code.FailedPrecondition), "not_attached", "This pull request is no longer attached to this session."],
    [new ConnectError("Pull Request is already merged", Code.Internal), "already_merged", "Pull Request is already merged"],
    [new ConnectError("bad token", Code.Unauthenticated), "auth", "GitHub rejected the request (auth)"],
    [new ConnectError("slow down until 14:05", Code.ResourceExhausted), "rate_limited", "slow down until 14:05"],
    [new ConnectError("down", Code.Unavailable), "network", "Couldn't reach the server"],
    [new ConnectError("url must be a GitHub pull request URL", Code.InvalidArgument), "invalid_url", "url must be a GitHub pull request URL"],
    [new ConnectError("could not find x", Code.NotFound), "not_found", "could not find x"],
    [new ConnectError("checks failing", Code.Internal), "other", "checks failing"],
  ])("classify_should_mapCodeAndMessage_%#", (err, kind, message) => {
    expect(classifyPrError(err)).toEqual({ ok: false, kind, message });
  });

  it("classifyMessage_should_treatSuccessFalseMessagesLikeErrors", () => {
    expect(classifyPrMessage("PR was already merged")).toMatchObject({ kind: "already_merged" });
  });
});
