import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { FeatureFlagsProvider } from "@/lib/contexts/FeatureFlagsContext";
import { resetFlagStoreForTest } from "@/lib/features/flagStore";
import { FeatureGuard } from "./FeatureGuard";

const mockReplace = jest.fn();
const mockGetFeatureFlags = jest.fn();

jest.mock("@/lib/nav", () => ({
  useRouter: () => ({ replace: mockReplace, push: jest.fn() }),
  usePathname: () => "/changelog/",
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ getFeatureFlags: mockGetFeatureFlags }),
}));

function renderGuard(flag: "backlog" | "changelog") {
  return render(
    <FeatureFlagsProvider>
      <FeatureGuard flag={flag}>
        <span>guarded</span>
      </FeatureGuard>
    </FeatureFlagsProvider>,
  );
}

describe("FeatureGuard", () => {
  beforeEach(() => {
    resetFlagStoreForTest();
    mockReplace.mockClear();
    mockGetFeatureFlags.mockReset();
  });

  it("renders children when its own flag is on", async () => {
    mockGetFeatureFlags.mockResolvedValue({ flags: [{ name: "changelog", enabled: true, scopes: [] }] });
    renderGuard("changelog");
    expect(await screen.findByText("guarded")).toBeInTheDocument();
    expect(mockReplace).not.toHaveBeenCalled();
  });

  it("replaces to / once and renders nothing when its flag is off, even if another flag is on", async () => {
    mockGetFeatureFlags.mockResolvedValue({
      flags: [
        { name: "backlog", enabled: true, scopes: [] },
        { name: "changelog", enabled: false, scopes: [] },
      ],
    });
    const { container } = renderGuard("changelog");
    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith("/"));
    expect(mockReplace).toHaveBeenCalledTimes(1);
    expect(container).toBeEmptyDOMElement();
  });

  it("keeps the route's <title> while flags load, without rendering any guarded content", async () => {
    mockGetFeatureFlags.mockReturnValue(new Promise(() => {}));
    const { container } = renderGuard("changelog");
    await waitFor(() => expect(document.title).toBe("Stapler Squad Sessions"));
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByText("guarded")).not.toBeInTheDocument();
  });
});
