import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { BackgroundModelsSettings } from "./BackgroundModelsSettings";

const mockGet = jest.fn();
const mockUpdate = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ getBackgroundModels: mockGet, updateBackgroundModels: mockUpdate }),
}));
jest.mock("@connectrpc/connect-web", () => ({ createConnectTransport: () => ({}) }));
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://x" }));

jest.mock("./TaggingClassifierSettings.css", () => {
  return new Proxy({}, { get: (_t, prop) => (typeof prop === "string" ? prop : "") });
});

const loaded = {
  settings: { features: {}, stages: {}, effort: "" },
  featureDefaults: { "handoff-summary": "haiku" },
  stageDefaults: { work: "sonnet" },
  effortLevels: ["low", "medium"],
};

beforeEach(() => {
  mockGet.mockReset().mockResolvedValue(loaded);
  mockUpdate.mockReset().mockResolvedValue({});
});

test("shows defaults as placeholders and saves overrides live", async () => {
  render(<BackgroundModelsSettings />);
  const input = await screen.findByLabelText("handoff-summary");
  expect(input).toHaveAttribute("placeholder", "haiku");
  expect(screen.getByLabelText("work")).toHaveAttribute("placeholder", "sonnet");

  fireEvent.change(input, { target: { value: "sonnet" } });
  fireEvent.change(screen.getByLabelText(/Effort/), { target: { value: "low" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));

  await waitFor(() => expect(mockUpdate).toHaveBeenCalled());
  expect(mockUpdate.mock.calls[0][0].settings).toEqual({
    features: { "handoff-summary": "sonnet" },
    stages: {},
    effort: "low",
  });
  expect(await screen.findByRole("status")).toHaveTextContent(/no restart/);
});

test("shows a retryable error when load fails", async () => {
  mockGet.mockRejectedValueOnce(new Error("boom"));
  render(<BackgroundModelsSettings />);
  expect(await screen.findByRole("alert")).toHaveTextContent(/Couldn't load/);
});
