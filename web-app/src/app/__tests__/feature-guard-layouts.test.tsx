/**
 * Characterises the feature-flag guard layouts (Story 1.4 / 1.7): render null while flags
 * load, render null and `router.replace("/")` once when the flag is off, render children when on.
 * Runs against the real FeatureFlagsProvider with a mocked RPC client.
 */
import React from "react";
import { render, screen, waitFor, act } from "@testing-library/react";
import { FeatureFlagsProvider } from "@/lib/contexts/FeatureFlagsContext";
import { resetFlagStoreForTest } from "@/lib/features/flagStore";
import { ROUTE_META } from "../route-meta";
import BacklogLayout from "../backlog/layout";
import BacklogSourcesSettingsLayout from "../settings/backlog-sources/layout";
import ChangelogLayout from "../changelog/layout";

const mockReplace = jest.fn();
let mockPathname = "/";
const mockGetFeatureFlags = jest.fn();

jest.mock("@/lib/nav", () => ({
  ...jest.requireActual("@/lib/nav/impl.memory"),
  usePathname: () => mockPathname,
  useRouter: () => ({ replace: mockReplace, push: jest.fn() }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
// The guard is under test, not the stream; the real provider needs a redux store.
jest.mock("@/components/backlog/BacklogStreamContext", () => ({
  BacklogStreamProvider: ({ children }: { children: React.ReactNode }) => children,
}));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ getFeatureFlags: mockGetFeatureFlags }),
}));

const cases: Array<[string, string, string, React.ComponentType<{ children: React.ReactNode }>]> = [
  ["backlog/layout.tsx", "backlog", "/backlog", BacklogLayout],
  ["settings/backlog-sources/layout.tsx", "backlog", "/settings/backlog-sources", BacklogSourcesSettingsLayout],
  ["changelog/layout.tsx", "changelog", "/changelog", ChangelogLayout],
];

function renderLayout(Layout: React.ComponentType<{ children: React.ReactNode }>) {
  return render(
    <FeatureFlagsProvider>
      <Layout>
        <span>guarded child</span>
      </Layout>
    </FeatureFlagsProvider>,
  );
}

describe.each(cases)("%s", (_file, flag, path, Layout) => {
  beforeEach(() => {
    resetFlagStoreForTest();
    mockPathname = path;
    mockReplace.mockClear();
    mockGetFeatureFlags.mockReset();
  });

  it("renders null and does not redirect while flags are loading", async () => {
    let resolve!: (v: unknown) => void;
    mockGetFeatureFlags.mockReturnValue(new Promise((r) => { resolve = r; }));
    const { container } = renderLayout(Layout);

    expect(container).toBeEmptyDOMElement();
    expect(document.head.querySelector("title")?.textContent).toBe((ROUTE_META as Record<string, { title: string }>)[path].title);
    expect(mockReplace).not.toHaveBeenCalled();

    await act(async () => { resolve({ flags: [{ name: flag, enabled: true, scopes: [] }] }); });
  });

  it("renders null and replaces to / exactly once when the flag is off", async () => {
    mockGetFeatureFlags.mockResolvedValue({ flags: [{ name: flag, enabled: false, scopes: [] }] });
    const { container } = renderLayout(Layout);

    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith("/"));
    expect(mockReplace).toHaveBeenCalledTimes(1);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders children when the flag is on", async () => {
    mockGetFeatureFlags.mockResolvedValue({ flags: [{ name: flag, enabled: true, scopes: [] }] });
    renderLayout(Layout);

    expect(await screen.findByText("guarded child")).toBeInTheDocument();
    expect(mockReplace).not.toHaveBeenCalled();
  });
});
