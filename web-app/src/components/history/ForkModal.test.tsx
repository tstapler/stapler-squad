import { render, screen, fireEvent } from "@testing-library/react";
import { ForkModal } from "./ForkModal";
import type { ClaudeHistoryEntry } from "@/gen/session/v1/session_pb";

jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({ useSessionRepoPaths: () => [] }));
jest.mock("@/lib/hooks/useGitHubEnterpriseHosts", () => ({ useGitHubEnterpriseHosts: () => ({ hosts: [] }) }));
jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));
jest.mock("@/lib/hooks/useRepoPathSuggestions", () => ({
  ...jest.requireActual("@/lib/hooks/useRepoPathSuggestions"),
  useRepoPathSuggestions: () => ({ resolutions: new Map(), version: 0 }),
}));

const entry = { id: "abcdef123456", name: "My conversation", project: "/repo/app" } as ClaudeHistoryEntry;

beforeAll(() => {
  // jsdom lacks <dialog> methods
  HTMLDialogElement.prototype.showModal = jest.fn();
  HTMLDialogElement.prototype.close = jest.fn();
});

describe("ForkModal", () => {
  it("renders the directory as a RepoPathInput combobox prefilled from the entry", () => {
    render(<ForkModal entry={entry} submitting={false} error={null} onClose={jest.fn()} onSubmit={jest.fn()} />);
    const dir = screen.getByLabelText("Directory");
    expect(dir).toHaveAttribute("role", "combobox");
    expect(dir).toHaveValue("/repo/app");
  });

  it("propagates edits to the directory field", () => {
    render(<ForkModal entry={entry} submitting={false} error={null} onClose={jest.fn()} onSubmit={jest.fn()} />);
    const dir = screen.getByLabelText("Directory");
    fireEvent.change(dir, { target: { value: "/other" } });
    expect(dir).toHaveValue("/other");
  });
});
