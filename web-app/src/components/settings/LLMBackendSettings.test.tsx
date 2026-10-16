import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { LLMBackendSettings } from "./LLMBackendSettings";

const getSettings = jest.fn();
const updateSettings = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ getLLMBackendSettings: getSettings, updateLLMBackendSettings: updateSettings }),
}));
jest.mock("@connectrpc/connect-web", () => ({ createConnectTransport: () => ({}) }));
jest.mock("./LLMBackendSettings.css", () => new Proxy({}, { get: (_t, prop) => (typeof prop === "string" ? prop : "") }));
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://test" }));

const backend = (name: string, available: boolean) => ({
  name,
  available,
  supportsResume: name === "claude",
  supportsSystemPrompt: true,
  supportsToolRestriction: name === "claude",
  supportsWorkdir: true,
});

beforeEach(() => {
  getSettings.mockResolvedValue({
    settings: { defaultBackend: "", perFeature: {}, consoletteBaseUrl: "", anthropicBaseUrl: "", modelMaps: [] },
    backends: [backend("claude", true), backend("consolette", false)],
    featureKeys: ["summarize", "session-tagging"],
  });
  updateSettings.mockResolvedValue({});
});

test("saves default and per-feature overrides", async () => {
  render(<LLMBackendSettings />);
  await screen.findByLabelText("Default backend");

  fireEvent.change(screen.getByLabelText("session-tagging"), { target: { value: "consolette" } });
  fireEvent.change(screen.getByLabelText("Default backend"), { target: { value: "claude" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));

  await waitFor(() => expect(updateSettings).toHaveBeenCalled());
  const sent = updateSettings.mock.calls[0][0].settings;
  expect(sent.defaultBackend).toBe("claude");
  expect(sent.perFeature).toEqual({ "session-tagging": "consolette" });
  await screen.findByRole("status");
});

test("shows backend availability in the options", async () => {
  render(<LLMBackendSettings />);
  await screen.findByLabelText("Default backend");
  expect(screen.getAllByText("consolette (unavailable)").length).toBeGreaterThan(0);
});

test("shows an error with retry when load fails", async () => {
  getSettings.mockRejectedValueOnce(new Error("boom"));
  render(<LLMBackendSettings />);
  expect(await screen.findByRole("alert")).toHaveTextContent("Couldn't load");
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  await screen.findByLabelText("Default backend");
});
