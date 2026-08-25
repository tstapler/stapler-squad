import { ConnectError, Code } from "@connectrpc/connect";
import {
  jitteredDelay,
  BackoffState,
  isRetriableCloseCode,
  getWsCloseCode,
  isNonRetriableConnectError,
  connectTimeoutMs,
  FOREGROUND_CONNECT_TIMEOUT_MS,
  CONNECT_TIMEOUT_MS,
  FOREGROUND_FAST_ATTEMPTS,
} from "@/lib/utils/backoff";

describe("jitteredDelay", () => {
  it("jitteredDelay_should_returnValueBetweenZeroAndCap_When_attemptIsZero", () => {
    // attempt=0: ceiling = min(30000, 1000 * 2^0) = min(30000, 1000) = 1000
    // result must be in [0, 1000]
    const baseMs = 1000;
    const capMs = 30_000;
    for (let i = 0; i < 50; i++) {
      const delay = jitteredDelay(baseMs, capMs, 0);
      expect(delay).toBeGreaterThanOrEqual(0);
      expect(delay).toBeLessThanOrEqual(baseMs);
    }
  });

  it("jitteredDelay_should_neverExceedCapMs_When_attemptIsVeryLarge", () => {
    const baseMs = 1000;
    const capMs = 30_000;
    for (let i = 0; i < 100; i++) {
      const delay = jitteredDelay(baseMs, capMs, 50);
      expect(delay).toBeGreaterThanOrEqual(0);
      expect(delay).toBeLessThanOrEqual(capMs);
    }
  });

  it("jitteredDelay_should_haveMeanApproximatelyHalfCap_When_calledThousandTimes", () => {
    // At attempt=10 with base=1000, cap=30000:
    // ceiling = min(30000, 1000 * 2^10) = min(30000, 1024000) = 30000
    // mean of uniform(0, 30000) ≈ 15000
    const baseMs = 1000;
    const capMs = 30_000;
    const N = 1000;
    let sum = 0;
    for (let i = 0; i < N; i++) {
      sum += jitteredDelay(baseMs, capMs, 10);
    }
    const mean = sum / N;
    const expected = capMs / 2;
    // ±10% tolerance
    expect(mean).toBeGreaterThan(expected * 0.9);
    expect(mean).toBeLessThan(expected * 1.1);
  });
});

describe("BackoffState", () => {
  it("BackoffState_should_returnBaseRangeDelay_When_resetCalledBeforeNext", () => {
    const state = new BackoffState(1000, 30_000);
    // Advance several times
    state.next();
    state.next();
    state.next();
    expect(state.attempt).toBe(3);

    // Reset and verify next() returns value in [0, baseMs]
    state.reset();
    expect(state.attempt).toBe(0);

    // After reset, attempt=0 so ceiling = min(30000, 1000) = 1000
    for (let i = 0; i < 50; i++) {
      state.reset();
      const delay = state.next();
      expect(delay).toBeGreaterThanOrEqual(0);
      expect(delay).toBeLessThanOrEqual(1000);
    }
  });

  it("should increment attempt counter after each next() call", () => {
    const state = new BackoffState(1000, 30_000);
    expect(state.attempt).toBe(0);
    state.next();
    expect(state.attempt).toBe(1);
    state.next();
    expect(state.attempt).toBe(2);
  });
});

describe("isRetriableCloseCode", () => {
  it("isRetriableCloseCode_should_returnFalse_When_codeIs4001", () => {
    expect(isRetriableCloseCode(4001)).toBe(false);
  });

  it("isRetriableCloseCode_should_returnFalse_When_codeIs4004", () => {
    expect(isRetriableCloseCode(4004)).toBe(false);
  });

  it("isRetriableCloseCode_should_returnTrue_When_codeIs1006", () => {
    expect(isRetriableCloseCode(1006)).toBe(true);
  });

  it("isRetriableCloseCode_should_returnTrue_When_codeIs1000", () => {
    expect(isRetriableCloseCode(1000)).toBe(true);
  });
});

describe("getWsCloseCode", () => {
  it("getWsCloseCode_should_returnCode_When_connectErrorHasWsCloseCodeHeader", () => {
    const err = new ConnectError(
      "WebSocket closed",
      Code.Unavailable,
      new Headers({ "ws-close-code": "4001" })
    );
    expect(getWsCloseCode(err)).toBe(4001);
  });

  it("getWsCloseCode_should_returnNull_When_errorIsPlainError", () => {
    const err = new Error("boom");
    expect(getWsCloseCode(err)).toBeNull();
  });

  it("should return null when ConnectError has no ws-close-code header", () => {
    const err = new ConnectError("some error", Code.Unavailable);
    expect(getWsCloseCode(err)).toBeNull();
  });
});

describe("isNonRetriableConnectError", () => {
  it("isNonRetriableConnectError_should_returnTrue_When_wsCloseCodeHeaderIsNonRetriable", () => {
    const err = new ConnectError(
      "WebSocket closed",
      Code.Unavailable,
      new Headers({ "ws-close-code": "4001" })
    );
    expect(isNonRetriableConnectError(err)).toBe(true);
  });

  it("isNonRetriableConnectError_should_returnFalse_When_wsCloseCodeHeaderIsRetriable", () => {
    const err = new ConnectError(
      "WebSocket closed",
      Code.Unavailable,
      new Headers({ "ws-close-code": "1006" })
    );
    expect(isNonRetriableConnectError(err)).toBe(false);
  });

  it("isNonRetriableConnectError_should_returnTrue_When_nativeTransportErrorCodeIsUnauthenticated", () => {
    // createConnectTransport never sets a ws-close-code header — this is the
    // native-transport equivalent of ws-close-code 4001 (auth failure).
    const err = new ConnectError("unauthenticated", Code.Unauthenticated);
    expect(getWsCloseCode(err)).toBeNull();
    expect(isNonRetriableConnectError(err)).toBe(true);
  });

  it("isNonRetriableConnectError_should_returnTrue_When_nativeTransportErrorCodeIsNotFound", () => {
    // Native-transport equivalent of ws-close-code 4004 (session not found).
    const err = new ConnectError("session not found", Code.NotFound);
    expect(getWsCloseCode(err)).toBeNull();
    expect(isNonRetriableConnectError(err)).toBe(true);
  });

  it("isNonRetriableConnectError_should_returnFalse_When_nativeTransportErrorCodeIsRetriable", () => {
    const err = new ConnectError("unavailable", Code.Unavailable);
    expect(isNonRetriableConnectError(err)).toBe(false);
  });

  it("isNonRetriableConnectError_should_returnFalse_When_errorIsNotAConnectError", () => {
    expect(isNonRetriableConnectError(new Error("boom"))).toBe(false);
  });
});

describe("connectTimeoutMs", () => {
  it("connectTimeoutMs_should_returnForegroundTimeout_When_foregroundTrueAndAttemptZero", () => {
    expect(connectTimeoutMs(true, 0)).toBe(FOREGROUND_CONNECT_TIMEOUT_MS);
  });
  it("connectTimeoutMs_should_returnForegroundTimeout_When_foregroundTrueAndAttemptOne", () => {
    expect(connectTimeoutMs(true, 1)).toBe(FOREGROUND_CONNECT_TIMEOUT_MS);
  });
  it("connectTimeoutMs_should_returnNormalTimeout_When_foregroundTrueAndAttemptEqualsFastAttempts", () => {
    expect(connectTimeoutMs(true, FOREGROUND_FAST_ATTEMPTS)).toBe(CONNECT_TIMEOUT_MS);
  });
  it("connectTimeoutMs_should_returnNormalTimeout_When_foregroundFalseRegardlessOfAttempt", () => {
    expect(connectTimeoutMs(false, 0)).toBe(CONNECT_TIMEOUT_MS);
    expect(connectTimeoutMs(false, 5)).toBe(CONNECT_TIMEOUT_MS);
  });
});
