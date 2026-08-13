/**
 * Tests for useExportRules hook.
 * Covers UT-FE-05 through UT-FE-07.
 */

import { renderHook, act } from "@testing-library/react";
import { useExportRules } from "@/lib/hooks/useExportRules";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockExportRules = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    exportRules: mockExportRules,
  }),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: jest.fn().mockReturnValue({}),
}));

jest.mock("@bufbuild/protobuf", () => ({
  create: (_schema: unknown, fields: Record<string, unknown> = {}) => ({ ...fields }),
}));

// ---------------------------------------------------------------------------
// Browser API mocks
// ---------------------------------------------------------------------------

const mockCreateObjectURL = jest.fn(() => "blob:mock-url");
const mockRevokeObjectURL = jest.fn();
const mockClick = jest.fn();

let capturedAnchor: Partial<HTMLAnchorElement> = {};

// Use jest.fn to create a controlled anchor element, using real document for other tags.
const realCreateElement = document.createElement.bind(document);

beforeAll(() => {
  Object.defineProperty(global, "URL", {
    value: {
      createObjectURL: mockCreateObjectURL,
      revokeObjectURL: mockRevokeObjectURL,
    },
    writable: true,
  });

  jest.spyOn(document, "createElement").mockImplementation((tag: string, ...args: unknown[]) => {
    if (tag === "a") {
      const a = {
        href: "",
        download: "",
        click: mockClick,
      } as unknown as HTMLAnchorElement;
      capturedAnchor = a;
      return a;
    }
    return realCreateElement(tag as keyof HTMLElementTagNameMap, ...args as []);
  });
});

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useExportRules", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    capturedAnchor = {};
  });

  it("UT-FE-05: useExportRules_triggers_file_download", async () => {
    mockExportRules.mockResolvedValueOnce({ yamlContent: "rules:\n- name: test\n" });

    const { result } = renderHook(() => useExportRules());

    await act(async () => {
      await result.current.exportRules();
    });

    expect(mockCreateObjectURL).toHaveBeenCalledTimes(1);
    expect(mockClick).toHaveBeenCalledTimes(1);
    expect(capturedAnchor.download).toBe("rules.yaml");
    expect(mockRevokeObjectURL).toHaveBeenCalledWith("blob:mock-url");
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it("UT-FE-06: useExportRules_passes_ruleIds_filter", async () => {
    mockExportRules.mockResolvedValueOnce({ yamlContent: "rules: []\n" });

    const { result } = renderHook(() => useExportRules());

    await act(async () => {
      await result.current.exportRules(["id-1", "id-2"]);
    });

    // The mock `create` returns the fields object, so the call args include ruleIds.
    const callArg = mockExportRules.mock.calls[0][0];
    expect(callArg.ruleIds).toEqual(["id-1", "id-2"]);
  });

  it("UT-FE-07: useExportRules_sets_error_on_rpc_failure", async () => {
    mockExportRules.mockRejectedValueOnce(new Error("Export failed"));

    const { result } = renderHook(() => useExportRules());

    await act(async () => {
      await result.current.exportRules();
    });

    expect(result.current.error?.message).toBe("Export failed");
    expect(mockClick).not.toHaveBeenCalled();
    expect(mockCreateObjectURL).not.toHaveBeenCalled();
  });
});
