/**
 * Regression tests for useBacklogService's listBacklogItems filter
 * passthrough — specifically includeArchived, added alongside the backend
 * ExcludeDone/ExcludeArchived split so archived items can be excluded from
 * the default backlog view independently of "done" items.
 */

import { renderHook, act } from "@testing-library/react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { useBacklogService } from "@/lib/hooks/useBacklogService";

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({})),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
  createAuthInterceptor: () => () => ({}),
}));

describe("useBacklogService listBacklogItems includeArchived passthrough", () => {
  let listBacklogItemsMock: jest.Mock;

  beforeEach(() => {
    jest.clearAllMocks();
    listBacklogItemsMock = jest.fn().mockResolvedValue({ items: [] });
    (createClient as jest.Mock).mockReturnValue({ listBacklogItems: listBacklogItemsMock });
    (createConnectTransport as jest.Mock).mockReturnValue({});
  });

  it("listBacklogItems_should_DefaultIncludeArchivedFalse_When_NotSpecified", async () => {
    const { result } = renderHook(() => useBacklogService());

    await act(async () => {
      await result.current.listBacklogItems({ includeTerminal: true });
    });

    expect(listBacklogItemsMock).toHaveBeenCalledWith(
      expect.objectContaining({ includeArchived: false })
    );
  });

  it("listBacklogItems_should_PassIncludeArchivedTrue_When_CallerRequestsIt", async () => {
    const { result } = renderHook(() => useBacklogService());

    await act(async () => {
      await result.current.listBacklogItems({ includeTerminal: true, includeArchived: true });
    });

    expect(listBacklogItemsMock).toHaveBeenCalledWith(
      expect.objectContaining({ includeArchived: true })
    );
  });
});
