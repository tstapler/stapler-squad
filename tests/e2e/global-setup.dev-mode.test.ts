import { FullConfig } from "@playwright/test";

// startDevStack is mocked so this stays a fast, hermetic unit test — no
// real backend/next-dev process is ever spawned (REQ-6 #1/#2, validation.md).
const mockStartDevStack = jest.fn();

jest.mock("../../scripts/dev-stack/launch", () => ({
  startDevStack: (...args: unknown[]) => mockStartDevStack(...args),
}));

// global-setup.dev-mode.ts's storageState-fixture-rewriting block does a
// real fs.writeFileSync against tests/e2e/fixtures/*.json; mock fs so this
// unit test never mutates real files on disk.
jest.mock("fs", () => ({
  writeFileSync: jest.fn(),
}));

describe("global-setup.dev-mode", () => {
  const ORIGINAL_TEST_SERVER_URL = process.env.TEST_SERVER_URL;

  beforeEach(() => {
    jest.resetModules();
    mockStartDevStack.mockReset();
    delete process.env.TEST_SERVER_URL;
  });

  afterAll(() => {
    if (ORIGINAL_TEST_SERVER_URL === undefined) {
      delete process.env.TEST_SERVER_URL;
    } else {
      process.env.TEST_SERVER_URL = ORIGINAL_TEST_SERVER_URL;
    }
  });

  it("global-setup.dev-mode should set process.env.TEST_SERVER_URL to the frontend URL returned by startDevStack, not the backend URL", async () => {
    mockStartDevStack.mockResolvedValue({
      backendUrl: "http://localhost:54211",
      frontendUrl: "http://localhost:54212",
      stop: jest.fn(),
    });

    const globalSetup = (await import("./global-setup.dev-mode")).default;
    await globalSetup({} as FullConfig);

    expect(process.env.TEST_SERVER_URL).toBe("http://localhost:54212");
    expect(process.env.TEST_SERVER_URL).not.toBe("http://localhost:54211");
  });

  it("global-setup.dev-mode should rethrow when startDevStack rejects, rather than swallowing the error and leaving TEST_SERVER_URL unset", async () => {
    mockStartDevStack.mockRejectedValue(new Error("boom: backend failed to start"));

    const globalSetup = (await import("./global-setup.dev-mode")).default;

    await expect(globalSetup({} as FullConfig)).rejects.toThrow("boom: backend failed to start");
    expect(process.env.TEST_SERVER_URL).toBeUndefined();
  });
});
