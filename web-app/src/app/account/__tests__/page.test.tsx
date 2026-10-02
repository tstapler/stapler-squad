import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import AccountPage from "../page";

jest.mock("next/navigation", () => ({ useRouter: () => ({ replace: jest.fn() }) }));
jest.mock("@/lib/analytics/usePageView", () => ({ usePageView: () => {} }));
jest.mock("@/lib/contexts/AuthContext", () => ({
  useAuth: () => ({ authEnabled: true, authenticated: true, loading: false }),
}));
jest.mock("@/lib/auth/passkey", () => ({
  generateInvite: jest.fn(),
  listCredentials: jest.fn().mockResolvedValue([]),
  revokeCredential: jest.fn(),
}));
jest.mock("../account.css", () => new Proxy({}, { get: (_t, p) => (typeof p === "string" ? p : "") }));

describe("AccountPage add-device modal", () => {
  it("exposes the device-name input via an accessible name, not only a placeholder", async () => {
    render(<AccountPage />);
    fireEvent.click(await screen.findByRole("button", { name: "Add New Device" }));
    await waitFor(() => expect(screen.getByLabelText("Device name")).toBeInTheDocument());
  });
});
