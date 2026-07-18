import { getApiBaseUrl } from "../config";

describe("getApiBaseUrl", () => {
  const originalEnvValue = process.env.NEXT_PUBLIC_API_URL;

  afterEach(() => {
    if (originalEnvValue === undefined) {
      delete process.env.NEXT_PUBLIC_API_URL;
    } else {
      process.env.NEXT_PUBLIC_API_URL = originalEnvValue;
    }
  });

  it("returns the override when NEXT_PUBLIC_API_URL is set, even with window defined", () => {
    // jsdom test environment always defines `window`; the override must
    // still win over the same-origin browser assumption (Epic 2.1).
    expect(typeof window).not.toBe("undefined");

    process.env.NEXT_PUBLIC_API_URL = "http://localhost:54211/api";

    expect(getApiBaseUrl()).toBe("http://localhost:54211/api");
  });

  it("falls back to window.location.origin + /api when NEXT_PUBLIC_API_URL is unset", () => {
    delete process.env.NEXT_PUBLIC_API_URL;

    expect(getApiBaseUrl()).toBe(window.location.origin + "/api");
  });
});
