import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import HistoryPage from "../page";

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn(), replace: jest.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));
jest.mock("@/lib/analytics/usePageView", () => ({ usePageView: () => {} }));
jest.mock("@/lib/contexts/AnalyticsContext", () => ({ useAnalytics: () => ({ track: jest.fn() }) }));
jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost",
  createAuthInterceptor: () => (next: unknown) => next,
}));
jest.mock("@connectrpc/connect-web", () => ({ createConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ listClaudeHistory: jest.fn().mockRejectedValue(new Error("boom")) }),
}));
jest.mock("../history.css", () => new Proxy({}, { get: (_t, p) => (typeof p === "string" ? p : "") }));

describe("HistoryPage load failure", () => {
  it("announces the error banner with role=alert", async () => {
    render(<HistoryPage />);
    const alert = await screen.findByRole("alert");
    await waitFor(() => expect(alert).toHaveTextContent(/Failed to load history/));
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });
});
