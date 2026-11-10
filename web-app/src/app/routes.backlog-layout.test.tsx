import { act, render, screen } from "@testing-library/react";
import { Suspense } from "react";
import { FeatureFlagsProvider } from "@/lib/contexts/FeatureFlagsContext";
import { resetFlagStoreForTest } from "@/lib/features/flagStore";
import { useBacklogStream } from "@/components/backlog/BacklogStreamContext";
import type { PageRoute } from "@/lib/nav/app-types";
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
    const Wrap = appRoutes.find((r): r is PageRoute => r.path === path && "load" in r)?.wrap;
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
    // Macrotasks drain the lazy() import, the mocked flag fetch and Suspense; no polling budget involved.
    for (let i = 0; i < 5 && !screen.queryByText("b"); i++) {
      await act(async () => void (await new Promise<void>((r) => setTimeout(r, 0))));
    }
    expect(screen.getByText("b")).toBeInTheDocument();
    expect(mockWatch).toHaveBeenCalledTimes(1);
  });
});
