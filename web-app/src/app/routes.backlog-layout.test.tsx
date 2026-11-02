import { render, screen, waitFor } from "@testing-library/react";
import { Suspense } from "react";
import { FeatureFlagsProvider } from "@/lib/contexts/FeatureFlagsContext";
import { resetFlagStoreForTest } from "@/lib/features/flagStore";
import { useBacklogStream } from "@/components/backlog/BacklogStreamContext";
import { appRoutes } from "./routes";

const mockWatch = jest.fn(() => ({ items: [], connectionState: "live" as const }));
jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({ useWatchBacklogItems: () => mockWatch() }));
jest.mock("@/lib/nav", () => ({ useRouter: () => ({ replace: jest.fn(), push: jest.fn() }), usePathname: () => "/backlog/" }));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ getFeatureFlags: async () => ({ flags: [{ name: "backlog", enabled: true, scopes: [] }] }) }),
}));

function Consumer({ label }: { label: string }) {
  useBacklogStream();
  return <span>{label}</span>;
}

// The Bun router loads pages only, so app/backlog/layout.tsx's shared stream must come from the route table.
describe.each(["/backlog", "/backlog/board"])("%s route wrap", (path) => {
  beforeEach(() => {
    resetFlagStoreForTest();
    mockWatch.mockClear();
  });

  it("gives every useBacklogStream consumer one shared watch stream", async () => {
    const Wrap = appRoutes.find((r) => r.path === path && "wrap" in r)?.wrap;
    if (!Wrap) throw new Error(`${path} has no wrap`);
    render(
      <FeatureFlagsProvider>
        <Suspense fallback={null}>
          <Wrap>
            <Consumer label="a" />
            <Consumer label="b" />
          </Wrap>
        </Suspense>
      </FeatureFlagsProvider>,
    );
    await waitFor(() => expect(screen.getByText("b")).toBeInTheDocument());
    expect(mockWatch).toHaveBeenCalledTimes(1);
  });
});
