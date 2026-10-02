import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import HelpPage from "../page";
import { loadDocs } from "@/lib/docs/docLoader";

jest.mock("@/lib/docs/docLoader", () => ({
  loadDocs: jest.fn(),
  buildFuseIndex: () => ({ search: () => [] }),
}));
jest.mock("react-markdown", () => ({ __esModule: true, default: ({ children }: { children: string }) => <div>{children}</div> }));
jest.mock("remark-gfm", () => ({ __esModule: true, default: () => {} }));
jest.mock("../help.css", () => new Proxy({}, { get: (_t, p) => (p === "sidebarLink" ? () => "" : typeof p === "string" ? p : "") }));

const mockLoadDocs = loadDocs as jest.Mock;

describe("HelpPage load failure", () => {
  it("shows an announced error with Retry, and recovers on retry", async () => {
    mockLoadDocs.mockRejectedValueOnce(new Error("offline"));
    mockLoadDocs.mockResolvedValueOnce([{ slug: "intro", title: "Intro", content: "Hello docs" }]);
    render(<HelpPage />);

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/Couldn.t load the documentation/);

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(screen.getByText("Hello docs")).toBeInTheDocument());
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
