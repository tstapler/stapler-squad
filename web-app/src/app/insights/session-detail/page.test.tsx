/**
 * Route smoke test for the durable standalone `/insights/session-detail?sessionId=`
 * route (Epic 1.4, Story 1.4.3 — replaces the `/insights/session/[sessionId]/`
 * dynamic route, which couldn't be pre-rendered under `output: "export"`).
 * Confirms the page reads `sessionId` from the `?sessionId=` search param and
 * passes it straight through to SessionDetailPageClient — the piece that
 * actually resolves client-side against the static bundle on a cold load,
 * mirroring `/sessions/summary`'s `__tests__/page.test.tsx`.
 */

import React from "react";
import { render } from "@testing-library/react";
import SessionDetailRoute from "./page";

const mockUseSearchParams = jest.fn();
jest.mock("next/navigation", () => ({
  useSearchParams: () => mockUseSearchParams(),
}));

const mockSessionDetailPageClient = jest.fn((_props: { sessionId: string }) => (
  <div data-testid="mock-session-detail-page-client" />
));
jest.mock("./SessionDetailPageClient", () => ({
  SessionDetailPageClient: (props: { sessionId: string }) => mockSessionDetailPageClient(props),
}));

function searchParamsWith(sessionId: string | null) {
  return { get: (key: string) => (key === "sessionId" ? sessionId : null) };
}

describe("SessionDetailRoute", () => {
  beforeEach(() => {
    mockSessionDetailPageClient.mockClear();
    mockUseSearchParams.mockReturnValue(searchParamsWith("abc123"));
  });

  it("renders SessionDetailPageClient with the sessionId taken from the sessionId search param", () => {
    render(<SessionDetailRoute />);

    expect(mockSessionDetailPageClient).toHaveBeenCalledWith(
      expect.objectContaining({ sessionId: "abc123" })
    );
  });

  it("passes an empty sessionId when the search param is absent, rather than throwing", () => {
    mockUseSearchParams.mockReturnValue(searchParamsWith(null));

    render(<SessionDetailRoute />);

    expect(mockSessionDetailPageClient).toHaveBeenCalledWith(
      expect.objectContaining({ sessionId: "" })
    );
  });
});
