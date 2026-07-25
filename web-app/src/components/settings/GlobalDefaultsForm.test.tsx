/**
 * Tests for GlobalDefaultsForm component.
 *
 * Covers:
 *  1. Renders the Max Concurrent Backlog Work Items input with the loaded value
 *  2. Submitting includes maxConcurrentBacklogWorkItems in the update payload
 *  3. Editing the field updates the submitted payload
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { GlobalDefaultsForm } from "./GlobalDefaultsForm";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

jest.mock("@connectrpc/connect");
jest.mock("@connectrpc/connect-web");
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost" }));

jest.mock("./GlobalDefaultsForm.css", () => {
  return new Proxy(
    {},
    { get: (_target, prop) => (typeof prop === "string" ? prop : "") }
  );
});

const mockGetSessionDefaults = jest.fn();
const mockUpdateGlobalDefaults = jest.fn();

const sampleDefaults = {
  program: "claude",
  oneOffBaseDir: "~/oneoff",
  newProjectBaseDir: "~/Projects",
  tags: [],
  envVars: {},
  cliFlags: "",
  maxAutoReworkIterations: 3,
  maxConcurrentBacklogWorkItems: 2,
};

beforeEach(() => {
  jest.clearAllMocks();
  mockGetSessionDefaults.mockResolvedValue({ defaults: sampleDefaults });
  mockUpdateGlobalDefaults.mockResolvedValue({ defaults: sampleDefaults });
  (createClient as jest.Mock).mockReturnValue({
    getSessionDefaults: mockGetSessionDefaults,
    updateGlobalDefaults: mockUpdateGlobalDefaults,
  });
  (createConnectTransport as jest.Mock).mockReturnValue({});
});

describe("GlobalDefaultsForm", () => {
  it("renders the Max Concurrent Backlog Work Items input with the loaded value", async () => {
    render(<GlobalDefaultsForm />);
    const input = await screen.findByLabelText("Max Concurrent Backlog Work Items");
    expect(input).toHaveValue(2);
  });

  it("submits the loaded maxConcurrentBacklogWorkItems value unchanged", async () => {
    render(<GlobalDefaultsForm />);
    await screen.findByLabelText("Max Concurrent Backlog Work Items");

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockUpdateGlobalDefaults).toHaveBeenCalledWith(
        expect.objectContaining({ maxConcurrentBacklogWorkItems: 2 })
      );
    });
  });

  it("submits an edited maxConcurrentBacklogWorkItems value", async () => {
    render(<GlobalDefaultsForm />);
    const input = await screen.findByLabelText("Max Concurrent Backlog Work Items");

    fireEvent.change(input, { target: { value: "5" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockUpdateGlobalDefaults).toHaveBeenCalledWith(
        expect.objectContaining({ maxConcurrentBacklogWorkItems: 5 })
      );
    });
  });
});
