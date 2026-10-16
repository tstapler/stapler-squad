import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import HelpPage from "../page";

// Real docLoader on purpose: it swallows per-file failures (Promise.allSettled), so the page must
// treat "zero docs loaded" as the failure signal.
jest.mock("react-markdown", () => ({ __esModule: true, default: ({ children }: { children: string }) => <div>{children}</div> }));
jest.mock("remark-gfm", () => ({ __esModule: true, default: () => {} }));
jest.mock("../help.css", () => new Proxy({}, { get: (_t, p) => (p === "sidebarLink" ? () => "" : typeof p === "string" ? p : "") }));

const okResponse = { ok: true, status: 200, text: () => Promise.resolve("# Intro\nHello docs") };
const notFound = { ok: false, status: 404, text: () => Promise.resolve("") };

describe("HelpPage load failure", () => {
  const realFetch = global.fetch;
  afterEach(() => { global.fetch = realFetch; });

  it("shows an announced error and a Retry when every doc 404s, then recovers on retry", async () => {
    const fetchMock = jest.fn().mockResolvedValue(notFound);
    global.fetch = fetchMock as unknown as typeof fetch;
    render(<HelpPage />);

    expect(await screen.findByRole("alert")).toHaveTextContent(/Couldn.t load the documentation/);

    fetchMock.mockResolvedValue(okResponse);
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(screen.getAllByText(/Hello docs/).length).toBeGreaterThan(0));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
