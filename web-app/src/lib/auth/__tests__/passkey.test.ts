import { getAuthStatus } from "../passkey";

// authBase() is not exported, so we exercise it indirectly through
// getAuthStatus(), asserting on the URL that `fetch` was called with.
describe("authBase (via getAuthStatus)", () => {
  const originalEnvValue = process.env.NEXT_PUBLIC_API_URL;
  const mockAuthStatus = {
    auth_enabled: true,
    has_credentials: true,
    authenticated: false,
    setup_active: false,
  };

  beforeEach(() => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => mockAuthStatus,
    }) as unknown as typeof fetch;
  });

  afterEach(() => {
    if (originalEnvValue === undefined) {
      delete process.env.NEXT_PUBLIC_API_URL;
    } else {
      process.env.NEXT_PUBLIC_API_URL = originalEnvValue;
    }
    jest.restoreAllMocks();
  });

  it("uses the derived override base when NEXT_PUBLIC_API_URL is set, even with window defined", async () => {
    // jsdom test environment always defines `window`; the override must
    // still win over the same-origin browser assumption (Epic 2.1).
    expect(typeof window).not.toBe("undefined");

    process.env.NEXT_PUBLIC_API_URL = "http://localhost:54211/api";

    await getAuthStatus();

    expect(global.fetch).toHaveBeenCalledWith(
      "http://localhost:54211/auth/status",
      expect.anything()
    );
  });

  it("falls back to window.location.origin + /auth when NEXT_PUBLIC_API_URL is unset", async () => {
    delete process.env.NEXT_PUBLIC_API_URL;

    await getAuthStatus();

    expect(global.fetch).toHaveBeenCalledWith(
      window.location.origin + "/auth/status",
      expect.anything()
    );
  });
});
