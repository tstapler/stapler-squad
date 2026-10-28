import React from "react";
import { render, waitFor } from "@testing-library/react";
import { FeatureFlagsProvider, useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import { useFeatureFlagValue } from "@/lib/hooks/useFeatureFlagValue";
import { resetFlagStoreForTest } from "@/lib/features/flagStore";

const mockGetFeatureFlags = jest.fn();

jest.mock("@/lib/nav", () => ({ useRouter: () => ({ replace: jest.fn() }) }));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    getFeatureFlags: (...a: unknown[]) => mockGetFeatureFlags(...a),
    updateFeatureFlag: jest.fn(),
  }),
}));

function Consumer({ id, kind }: { id: number; kind: "context" | "value" }) {
  const ctx = useFeatureFlag("x");
  const val = useFeatureFlagValue("x");
  const shown = kind === "context" ? ctx : val;
  return <span data-testid={`c${id}`}>{String(shown)}</span>;
}

describe("flag consumer fan-out", () => {
  beforeEach(() => {
    resetFlagStoreForTest();
    mockGetFeatureFlags.mockReset();
    mockGetFeatureFlags.mockResolvedValue({
      flags: [{ name: "x", enabled: true, description: "", statusDetail: "", scopes: [] }],
    });
  });

  it("flagFanout_should_IssueOneGetFeatureFlags_When_10ConsumersMountTogether", async () => {
    const { getAllByText } = render(
      <FeatureFlagsProvider>
        {Array.from({ length: 10 }, (_, i) => (
          <Consumer key={i} id={i} kind={i % 2 === 0 ? "context" : "value"} />
        ))}
      </FeatureFlagsProvider>,
    );
    await waitFor(() => expect(getAllByText("true")).toHaveLength(10));
    expect(mockGetFeatureFlags).toHaveBeenCalledTimes(1);
  });
});
