/**
 * Characterises the feature-flag guard on each guarded path (Story 1.4 / 1.7): render null while flags
 * load, render null and `router.replace("/")` once when the flag is off, render children when on.
 * Runs against the real FeatureFlagsProvider with a mocked RPC client.
 */
import React from "react";
import { render, screen, waitFor, act } from "@testing-library/react";
import { FeatureFlagsProvider } from "@/lib/contexts/FeatureFlagsContext";
import { resetFlagStoreForTest } from "@/lib/features/flagStore";
import { ROUTE_META } from "../route-meta";
import { FeatureGuard } from "../FeatureGuard";
import type { GuardFlag } from "../route-meta";

const mockReplace = jest.fn();
let mockPathname = "/";
const mockGetFeatureFlags = jest.fn();

jest.mock("@/lib/nav", () => ({
  ...jest.requireActual("@/lib/nav/impl.memory"),
  usePathname: () => mockPathname,
  useRouter: () => ({ replace: mockReplace, push: jest.fn() }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ getFeatureFlags: mockGetFeatureFlags }),
}));

const cases: Array<[string, GuardFlag, string]> = [
  ["backlog", "backlog", "/backlog"],
  ["settings/backlog-sources", "backlog", "/settings/backlog-sources"],
  ["changelog", "changelog", "/changelog"],
];

function renderGuarded(flag: GuardFlag) {
  return render(
    <FeatureFlagsProvider>
      <FeatureGuard flag={flag}>
        <span>guarded child</span>
      </FeatureGuard>
    </FeatureFlagsProvider>,
  );
}

describe.each(cases)("%s", (_name, flag, path) => {
  beforeEach(() => {
    resetFlagStoreForTest();
    mockPathname = path;
    mockReplace.mockClear();
    mockGetFeatureFlags.mockReset();
  });

  it("renders null and does not redirect while flags are loading", async () => {
    let resolve!: (v: unknown) => void;
    mockGetFeatureFlags.mockReturnValue(new Promise((r) => { resolve = r; }));
    const { container } = renderGuarded(flag);

    expect(container).toBeEmptyDOMElement();
    expect(document.head.querySelector("title")?.textContent).toBe((ROUTE_META as Record<string, { title: string }>)[path].title);
    expect(mockReplace).not.toHaveBeenCalled();

    await act(async () => { resolve({ flags: [{ name: flag, enabled: true, scopes: [] }] }); });
  });

  it("renders null and replaces to / exactly once when the flag is off", async () => {
    mockGetFeatureFlags.mockResolvedValue({ flags: [{ name: flag, enabled: false, scopes: [] }] });
    const { container } = renderGuarded(flag);

    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith("/"));
    expect(mockReplace).toHaveBeenCalledTimes(1);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders children when the flag is on", async () => {
    mockGetFeatureFlags.mockResolvedValue({ flags: [{ name: flag, enabled: true, scopes: [] }] });
    renderGuarded(flag);

    expect(await screen.findByText("guarded child")).toBeInTheDocument();
    expect(mockReplace).not.toHaveBeenCalled();
  });
});
