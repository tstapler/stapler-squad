/**
 * Tests for useApprovalResolution's resolveApproval error handling.
 *
 * Task 2.3.2a: verify a `ConnectError` with `code === Code.FailedPrecondition`
 * always lands in `blockedApprovals`, regardless of the message shape — this
 * already covers both the pre-existing CI-red-guard block (AC5) and the newer
 * "already auto-resolved by rule" mid-review-race message (Epic 2.3), since
 * both surface the same Connect error code. A non-FailedPrecondition error
 * (validation.md row 3) instead lands in `failedApprovals` with the retry
 * message, keeping the item actionable rather than dead-ending on "Expired".
 */

import { renderHook, act } from "@testing-library/react";
import { useApprovalResolution } from "./useApprovalResolution";

// ── Mocks ──────────────────────────────────────────────────────────────────

const mockResolveApproval = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    resolveApproval: mockResolveApproval,
  }),
  ConnectError: class ConnectError extends Error {
    code: number;
    rawMessage: string;
    constructor(message: string, code: number = 0) {
      super(message);
      this.name = "ConnectError";
      this.code = code;
      this.rawMessage = message;
    }
  },
  Code: { FailedPrecondition: 9 },
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
  createAuthInterceptor: () => (next: unknown) => next,
}));

jest.mock("@bufbuild/protobuf", () => ({
  create: (_schema: unknown, fields: Record<string, unknown> = {}) => ({ ...fields }),
}));

// Imported after the connect mock above so the class it constructs matches the
// mocked ConnectError the hook itself imports.
import { ConnectError, Code } from "@connectrpc/connect";

describe("useApprovalResolution — resolveApproval error handling", () => {
  beforeEach(() => {
    mockResolveApproval.mockReset();
  });

  function setup() {
    return renderHook(() =>
      useApprovalResolution({
        notificationHistory: [],
        acknowledgeNotification: jest.fn(),
      })
    );
  }

  it("routes a FailedPrecondition with a reconciliation message into blockedApprovals, not the generic expired path", async () => {
    const message = 'already auto-resolved by rule "Auto-allow safe git status checks" while you were reviewing it — no action needed';
    mockResolveApproval.mockRejectedValueOnce(new ConnectError(message, Code.FailedPrecondition));

    const { result } = setup();

    await act(async () => {
      await result.current.resolveApproval("appr-1", "allow", "notif-1");
    });

    expect(result.current.blockedApprovals["appr-1"]).toBe(message);
    expect(result.current.resolvedApprovals["appr-1"]).toBeUndefined();
  });

  it("still routes a FailedPrecondition with a CI-shaped message into blockedApprovals (regression)", async () => {
    const message = "Approval blocked: CI is failing on this branch — review before approving. https://github.com/org/repo/pull/1/checks";
    mockResolveApproval.mockRejectedValueOnce(new ConnectError(message, Code.FailedPrecondition));

    const { result } = setup();

    await act(async () => {
      await result.current.resolveApproval("appr-2", "allow", "notif-2");
    });

    expect(result.current.blockedApprovals["appr-2"]).toBe(message);
    expect(result.current.resolvedApprovals["appr-2"]).toBeUndefined();
  });

  it("routes a non-FailedPrecondition error into failedApprovals with the retry message, leaving the item actionable", async () => {
    mockResolveApproval.mockRejectedValueOnce(new Error("network error"));

    const { result } = setup();

    await act(async () => {
      await result.current.resolveApproval("appr-3", "allow", "notif-3");
    });

    expect(result.current.blockedApprovals["appr-3"]).toBeUndefined();
    expect(result.current.resolvedApprovals["appr-3"]).toBeUndefined();
    expect(result.current.failedApprovals["appr-3"]).toBe("Couldn't record your decision — try again.");
  });

  it("clears a prior failedApprovals entry on a subsequent successful retry", async () => {
    mockResolveApproval.mockRejectedValueOnce(new Error("network error"));
    mockResolveApproval.mockResolvedValueOnce({});

    const { result } = setup();

    await act(async () => {
      await result.current.resolveApproval("appr-5", "allow", "notif-5");
    });
    expect(result.current.failedApprovals["appr-5"]).toBe("Couldn't record your decision — try again.");

    await act(async () => {
      await result.current.resolveApproval("appr-5", "allow", "notif-5");
    });

    expect(result.current.failedApprovals["appr-5"]).toBeUndefined();
    expect(result.current.resolvedApprovals["appr-5"]).toBe("allow");
  });

  it("resolves normally on success", async () => {
    const acknowledgeNotification = jest.fn();
    mockResolveApproval.mockResolvedValueOnce({});

    const { result } = renderHook(() =>
      useApprovalResolution({
        notificationHistory: [],
        acknowledgeNotification,
      })
    );

    await act(async () => {
      await result.current.resolveApproval("appr-4", "allow", "notif-4");
    });

    expect(result.current.resolvedApprovals["appr-4"]).toBe("allow");
    expect(acknowledgeNotification).toHaveBeenCalledWith("notif-4");
  });
});
