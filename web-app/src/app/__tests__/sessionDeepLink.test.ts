import {
  classifyGetSessionFailure,
  parseSessionDeepLink,
  shouldResolveViaGetSession,
} from "../sessionDeepLink";

describe("session deep link helpers (Story 5.3)", () => {
  it("parse_should_read_session_tab_newpane_and_notification_params", () => {
    const link = parseSessionDeepLink(new URLSearchParams("session=h1&tab=terminal&notification=n9&newPane=true"));
    expect(link).toEqual({ sessionId: "h1", tab: "terminal", newPane: "true", notificationId: "n9" });
  });

  it("parse_should_return_nulls_for_a_bare_url", () => {
    expect(parseSessionDeepLink(new URLSearchParams(""))).toEqual({
      sessionId: null,
      tab: null,
      newPane: null,
      notificationId: null,
    });
  });

  it("should_resolve_via_getSession_when_the_list_settled_empty_and_id_is_absent", () => {
    expect(shouldResolveViaGetSession({ sessionId: "h1", foundInList: false, listSettled: true })).toBe(true);
  });

  it("should_wait_while_the_list_has_not_settled", () => {
    expect(shouldResolveViaGetSession({ sessionId: "h1", foundInList: false, listSettled: false })).toBe(false);
  });

  it("should_not_resolve_via_getSession_when_found_in_list_or_no_session_param", () => {
    expect(shouldResolveViaGetSession({ sessionId: "h1", foundInList: true, listSettled: true })).toBe(false);
    expect(shouldResolveViaGetSession({ sessionId: null, foundInList: false, listSettled: true })).toBe(false);
  });

  it("classify_should_separate_not_found_from_retryable_failure", () => {
    expect(classifyGetSessionFailure({ code: 5 })).toBe("not_found");
    expect(classifyGetSessionFailure({ code: 14 })).toBe("failed");
    expect(classifyGetSessionFailure(new Error("network"))).toBe("failed");
    expect(classifyGetSessionFailure(null)).toBe("failed");
  });
});
