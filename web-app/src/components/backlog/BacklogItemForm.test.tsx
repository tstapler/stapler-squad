/**
 * Tests for BacklogItemForm — repo-path UX fix plus the Epic 3.2 pipeline-mode
 * selector (see project_plans/backlog-configurable-pipeline/implementation/plan.md
 * and project_plans/backlog-configurable-pipeline/design/ux.md, G-1..G-4).
 *
 * Covers:
 *  1. Repository Path field renders its generic hint text
 *  2. Skip planning / skip review gate checkboxes render plain-language help text
 *  3. Typing a GitHub URL into Repository Path shows the "Will clone" confirmation
 *  4. Submitting with a GitHub URL in Repository Path shows a "Cloning repository…" busy label
 *  5. Pipeline mode: default state on new-item mount ("Default" selected)
 *  6. Pipeline mode: selecting a mode updates the onSubmit payload
 *  7. G-4: mode list still loading — only "Default" selectable, submit never blocked
 *  8. G-3: mode list fetch failure — role="status" notice, "Default" still selectable, still submittable
 *  9. G-2: item references a mode not in the fetched list — synthetic disabled "Unknown mode" option
 *  10. G-1: selected mode needs {{repo_path}} but item.repoPath is empty — non-blocking role="alert" warning
 */

import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { BacklogItemForm } from "./BacklogItemForm";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import type { PipelineMode } from "@/lib/hooks/useBacklogService";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";

// RepoPathInput uses useSessionRepoPaths (Redux) and usePathCompletions (RPC).
// Stub both so tests don't need a Redux store or ConnectRPC transport.
jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: () => [],
}));

jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));

// BacklogItemForm now calls useBacklogService() directly for listPipelineModes.
// Mock the whole hook so tests control the fetch's pending/resolved/rejected
// state deterministically, without a real ConnectRPC transport.
jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: jest.fn(),
}));

// backlog:sdd-default-pipeline pre-selection (project_plans/backlog-sdd-default-pipeline)
// reads useFeatureFlag directly rather than the whole flags map — mock it so
// each test controls the flag value deterministically without a real
// FeatureFlagsProvider/ConnectRPC transport.
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: jest.fn(() => false),
}));

const mockUseBacklogService = useBacklogService as jest.MockedFunction<typeof useBacklogService>;

function mockListPipelineModes(listPipelineModes: () => Promise<PipelineMode[]>) {
  mockUseBacklogService.mockReturnValue({
    listPipelineModes,
  } as unknown as ReturnType<typeof useBacklogService>);
}

function makeMode(overrides: Partial<PipelineMode> & Pick<PipelineMode, "slug" | "name">): PipelineMode {
  return {
    id: `id-${overrides.slug}`,
    description: "",
    enabled: true,
    statusCommandTemplate: "",
    doneCommandTemplate: "",
    failCommandTemplate: "",
    reviewCommandTemplate: "",
    shipCommandTemplate: "",
    helpCommandTemplate: "",
    triagePromptTemplate: "",
    reviewPromptTemplate: "",
    initialPromptTemplate: "",
    contentHash: "hash",
    ...overrides,
  };
}

const QUICK_MODE = makeMode({
  slug: "quick",
  name: "Quick Fix",
  description: "Fast, low-ceremony pipeline",
  triagePromptTemplate: "Fix {{item_id}} fast.",
});

const FULL_MODE = makeMode({
  slug: "full",
  name: "Full SDD",
  description: "Full SDD pipeline",
  triagePromptTemplate: "Use {{repo_path}} to run the test suite before triage.",
});

const SDD_MODE = makeMode({
  slug: "sdd",
  name: "SDD (Stapler-Driven Development)",
  description: "Multi-phase research/plan/validate/implement/verify pipeline",
});

const mockUseFeatureFlag = useFeatureFlag as jest.MockedFunction<typeof useFeatureFlag>;

beforeEach(() => {
  mockUseBacklogService.mockReset();
  // Default: resolves to an empty mode list so unrelated tests aren't affected.
  mockListPipelineModes(() => Promise.resolve([]));
  mockUseFeatureFlag.mockReset();
  mockUseFeatureFlag.mockReturnValue(false);
});

describe("BacklogItemForm — Repository Path help", () => {
  it("renders the generic repo-path hint", async () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    expect(
      screen.getByText("Local path to your clone, or a GitHub URL — we'll clone it for you.")
    ).toBeInTheDocument();
  });

  it("shows a clone confirmation when a GitHub URL is entered", async () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    fireEvent.change(screen.getByTestId("backlog-repo-path-input"), {
      target: { value: "https://github.com/facebook/react" },
    });

    expect(screen.getByTestId("repo-path-github-hint")).toHaveTextContent(
      "Will clone facebook/react"
    );
  });
});

describe("BacklogItemForm — checkbox help text", () => {
  it("explains skip planning phase in plain language", async () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    expect(
      screen.getByText("Go straight to triage without a separate planning pass.")
    ).toBeInTheDocument();
  });

  it("explains skip review gate in plain language", async () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    expect(
      screen.getByText("Mark work done without an automated review pass first.")
    ).toBeInTheDocument();
  });

  it("wraps the 4 checkboxes in an 'Overrides' fieldset", async () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    const fieldset = screen.getByTestId("backlog-overrides-fieldset");
    expect(fieldset.tagName).toBe("FIELDSET");
    expect(screen.getByText("Overrides (independent of pipeline mode)")).toBeInTheDocument();
    expect(fieldset).toContainElement(screen.getByTestId("backlog-skip-planning-checkbox"));
    expect(fieldset).toContainElement(screen.getByTestId("backlog-skip-review-checkbox"));
    expect(fieldset).toContainElement(screen.getByTestId("backlog-auto-spawn-session-checkbox"));
    expect(fieldset).toContainElement(screen.getByTestId("backlog-auto-create-pr-checkbox"));
  });

  it("explains auto-create-pr in plain language", async () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    expect(
      screen.getByText(
        /Skip the manual Review Queue "Create PR" click — a PR is opened automatically/
      )
    ).toBeInTheDocument();
  });
});

describe("BacklogItemForm — auto-create-pr toggle", () => {
  it("defaults to unchecked and submits autoCreatePR: false when left untouched", async () => {
    const onSubmit = jest.fn(() => Promise.resolve());
    render(<BacklogItemForm onSubmit={onSubmit} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    expect(screen.getByTestId("backlog-auto-create-pr-checkbox")).not.toBeChecked();

    fireEvent.change(screen.getByTestId("backlog-title-input"), {
      target: { value: "Some title" },
    });
    fireEvent.change(screen.getByTestId("backlog-repo-path-input"), {
      target: { value: "/home/user/project" },
    });
    fireEvent.click(screen.getByTestId("backlog-form-submit"));

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ autoCreatePR: false }))
    );
  });

  it("submits autoCreatePR: true once the checkbox is checked", async () => {
    const onSubmit = jest.fn(() => Promise.resolve());
    render(<BacklogItemForm onSubmit={onSubmit} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    fireEvent.click(screen.getByTestId("backlog-auto-create-pr-checkbox"));
    expect(screen.getByTestId("backlog-auto-create-pr-checkbox")).toBeChecked();

    fireEvent.change(screen.getByTestId("backlog-title-input"), {
      target: { value: "Some title" },
    });
    fireEvent.change(screen.getByTestId("backlog-repo-path-input"), {
      target: { value: "/home/user/project" },
    });
    fireEvent.click(screen.getByTestId("backlog-form-submit"));

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ autoCreatePR: true }))
    );
  });
});

describe("BacklogItemForm — cloning busy state", () => {
  it("shows 'Cloning repository…' instead of 'Saving…' while submitting a GitHub URL", async () => {
    let resolveSubmit: () => void = () => {};
    const onSubmit = jest.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveSubmit = resolve;
        })
    );

    render(<BacklogItemForm onSubmit={onSubmit} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    fireEvent.change(screen.getByTestId("backlog-title-input"), {
      target: { value: "Clone and triage this repo" },
    });
    fireEvent.change(screen.getByTestId("backlog-repo-path-input"), {
      target: { value: "https://github.com/facebook/react" },
    });
    fireEvent.click(screen.getByTestId("backlog-form-submit"));

    await waitFor(() =>
      expect(screen.getByTestId("backlog-form-submit")).toHaveTextContent("Cloning repository…")
    );

    resolveSubmit();
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
  });
});

describe("BacklogItemForm — description write/preview", () => {
  it("renders markdown in the preview tab and shows raw text in the write tab", async () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    fireEvent.change(screen.getByTestId("backlog-description-input"), {
      target: { value: "**bold text**" },
    });

    fireEvent.click(screen.getByTestId("backlog-description-tab-preview"));
    expect(screen.getByText("bold text").tagName).toBe("STRONG");

    fireEvent.click(screen.getByTestId("backlog-description-tab-write"));
    expect(screen.getByTestId("backlog-description-input")).toHaveValue("**bold text**");
  });

  it("renders a link with correct href in preview", async () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    fireEvent.change(screen.getByTestId("backlog-description-input"), {
      target: { value: "see [logo](https://example.com/logo.png)" },
    });
    fireEvent.click(screen.getByTestId("backlog-description-tab-preview"));

    const link = screen.getByRole("link", { name: "logo" });
    expect(link).toHaveAttribute("href", "https://example.com/logo.png");
  });

  it("never executes injected script tags in preview", async () => {
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    fireEvent.change(screen.getByTestId("backlog-description-input"), {
      target: { value: "<script>window.__pwned = true;</script>" },
    });
    fireEvent.click(screen.getByTestId("backlog-description-tab-preview"));

    expect(document.querySelector("script")).not.toBeInTheDocument();
    expect((window as unknown as { __pwned?: boolean }).__pwned).toBeUndefined();
  });
});

describe("BacklogItemForm — image attachment upload", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it("uploads an image and inserts a markdown image reference", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ path: "/home/user/.stapler-squad/backlog-attachments/1-screenshot.png", filename: "1-screenshot.png" }),
    });

    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    const file = new File(["fake-bytes"], "screenshot.png", { type: "image/png" });
    fireEvent.change(screen.getByTestId("backlog-attach-image-input"), {
      target: { files: [file] },
    });

    await waitFor(() =>
      expect(
        (screen.getByTestId("backlog-description-input") as HTMLTextAreaElement).value
      ).toContain("![1-screenshot.png]")
    );
    expect(global.fetch).toHaveBeenCalledWith(
      expect.stringContaining("/v1/upload-backlog-attachment"),
      expect.objectContaining({ method: "POST" })
    );
  });

  it("rejects a non-image file client-side without calling fetch", async () => {
    global.fetch = jest.fn();
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    const file = new File(["hello"], "notes.txt", { type: "text/plain" });
    fireEvent.change(screen.getByTestId("backlog-attach-image-input"), {
      target: { files: [file] },
    });

    expect(screen.getByTestId("backlog-attach-image-error")).toHaveTextContent(
      "Only image files can be attached."
    );
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it("rejects an oversized image client-side without calling fetch", async () => {
    global.fetch = jest.fn();
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    const bigFile = new File([new Uint8Array(11 * 1024 * 1024)], "big.png", { type: "image/png" });
    fireEvent.change(screen.getByTestId("backlog-attach-image-input"), {
      target: { files: [bigFile] },
    });

    expect(screen.getByTestId("backlog-attach-image-error")).toHaveTextContent(
      "Image is too large (max 10 MB)."
    );
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it("shows a server-side rejection message on a 415 response", async () => {
    global.fetch = jest.fn().mockResolvedValue({ ok: false, status: 415 });
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    const file = new File(["fake-bytes"], "photo.png", { type: "image/png" });
    fireEvent.change(screen.getByTestId("backlog-attach-image-input"), {
      target: { files: [file] },
    });

    await waitFor(() =>
      expect(screen.getByTestId("backlog-attach-image-error")).toHaveTextContent(
        "Unsupported image type"
      )
    );
  });

  it("shows a network-error message when fetch rejects", async () => {
    global.fetch = jest.fn().mockRejectedValue(new Error("boom"));
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    const file = new File(["fake-bytes"], "photo.png", { type: "image/png" });
    fireEvent.change(screen.getByTestId("backlog-attach-image-input"), {
      target: { files: [file] },
    });

    await waitFor(() =>
      expect(screen.getByTestId("backlog-attach-image-error")).toHaveTextContent(
        "Network error — upload failed."
      )
    );
  });

  it("shows a generic failure message on a non-413/415 error response", async () => {
    global.fetch = jest.fn().mockResolvedValue({ ok: false, status: 500 });
    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    const file = new File(["fake-bytes"], "photo.png", { type: "image/png" });
    fireEvent.change(screen.getByTestId("backlog-attach-image-input"), {
      target: { files: [file] },
    });

    await waitFor(() =>
      expect(screen.getByTestId("backlog-attach-image-error")).toHaveTextContent(
        "Upload failed."
      )
    );
  });

  it("uploads a pasted image and inserts a markdown image reference", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        path: "/home/user/.stapler-squad/backlog-attachments/2-pasted.png",
        filename: "2-pasted.png",
      }),
    });

    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);
    await screen.findByTestId("backlog-pipeline-mode-default");

    const file = new File(["fake-bytes"], "pasted.png", { type: "image/png" });
    const clipboardData = {
      items: [
        {
          type: "image/png",
          getAsFile: () => file,
        },
      ],
    };

    fireEvent.paste(screen.getByTestId("backlog-description-input"), { clipboardData });

    await waitFor(() =>
      expect(
        (screen.getByTestId("backlog-description-input") as HTMLTextAreaElement).value
      ).toContain("![2-pasted.png]")
    );
    expect(global.fetch).toHaveBeenCalledWith(
      expect.stringContaining("/v1/upload-backlog-attachment"),
      expect.objectContaining({ method: "POST" })
    );
  });
});

describe("BacklogItemForm — pipeline mode selector (Epic 3.2)", () => {
  // The jest styleMock for `.css.ts` files wraps every export (including plain
  // `style()` string exports) in a callable proxy function, which triggers a
  // benign "Invalid value for prop className" React warning when RadioGroup
  // renders. Scoped to just this describe block — the one that actually
  // exercises RadioGroup — so console.error output is not silenced for the
  // other describe blocks in this file. Pre-existing jest/vanilla-extract mock
  // limitation — see RadioGroup.test.tsx, which silences it the same way.
  beforeEach(() => {
    jest.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("defaults to 'Default' selected on a new-item mount", async () => {
    mockListPipelineModes(() => Promise.resolve([QUICK_MODE, FULL_MODE]));

    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    const defaultOption = await screen.findByTestId("backlog-pipeline-mode-default");
    expect(defaultOption).toHaveAttribute("aria-checked", "true");

    const quickOption = await screen.findByTestId("backlog-pipeline-mode-quick");
    expect(quickOption).toHaveAttribute("aria-checked", "false");
  });

  it("includes the selected mode slug in the onSubmit payload", async () => {
    mockListPipelineModes(() => Promise.resolve([QUICK_MODE, FULL_MODE]));
    const onSubmit = jest.fn().mockResolvedValue(undefined);

    render(<BacklogItemForm onSubmit={onSubmit} onCancel={jest.fn()} />);

    await screen.findByTestId("backlog-pipeline-mode-quick");
    fireEvent.click(screen.getByTestId("backlog-pipeline-mode-quick"));

    fireEvent.change(screen.getByTestId("backlog-title-input"), {
      target: { value: "Some title" },
    });
    fireEvent.change(screen.getByTestId("backlog-repo-path-input"), {
      target: { value: "/home/user/project" },
    });
    fireEvent.click(screen.getByTestId("backlog-form-submit"));

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ pipelineMode: "quick" }))
    );
  });

  it("G-4: only 'Default' is selectable while the mode fetch is pending, and submit is never blocked by it", async () => {
    // A promise that never resolves during this test — simulates "still loading".
    mockListPipelineModes(() => new Promise<PipelineMode[]>(() => {}));

    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    expect(screen.getByTestId("backlog-pipeline-mode-default")).toBeInTheDocument();
    expect(screen.queryByTestId("backlog-pipeline-mode-quick")).not.toBeInTheDocument();
    expect(screen.queryByTestId("backlog-pipeline-mode-full")).not.toBeInTheDocument();

    // Save button is not disabled on account of the pending mode fetch.
    expect(screen.getByTestId("backlog-form-submit")).not.toBeDisabled();
  });

  it("G-3: shows the fetch-failure notice, keeps 'Default' selectable, and stays submittable", async () => {
    mockListPipelineModes(() => Promise.reject(new Error("network error")));
    const onSubmit = jest.fn().mockResolvedValue(undefined);

    render(<BacklogItemForm onSubmit={onSubmit} onCancel={jest.fn()} />);

    const notice = await screen.findByTestId("backlog-pipeline-mode-fetch-error");
    expect(notice).toHaveAttribute("role", "status");
    expect(notice).toHaveTextContent(
      "Couldn't load pipeline modes — you can still save with Default."
    );

    const defaultOption = screen.getByTestId("backlog-pipeline-mode-default");
    expect(defaultOption).not.toBeDisabled();

    fireEvent.change(screen.getByTestId("backlog-title-input"), {
      target: { value: "Some title" },
    });
    fireEvent.change(screen.getByTestId("backlog-repo-path-input"), {
      target: { value: "/home/user/project" },
    });
    fireEvent.click(screen.getByTestId("backlog-form-submit"));

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ pipelineMode: "" }))
    );
  });

  it("shows a hint linking to Settings when the fetch succeeds but no pipeline modes exist yet", async () => {
    mockListPipelineModes(() => Promise.resolve([]));

    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    await screen.findByTestId("backlog-pipeline-mode-default");

    const hint = await screen.findByTestId("backlog-pipeline-mode-empty-hint");
    expect(hint).toHaveTextContent("No custom pipeline modes exist yet.");

    const link = within(hint).getByRole("link", { name: "Create one in Settings →" });
    expect(link).toHaveAttribute("href", "/settings/pipeline-modes");
  });

  it("does not show the empty-modes hint once real pipeline modes are available", async () => {
    mockListPipelineModes(() => Promise.resolve([QUICK_MODE, FULL_MODE]));

    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    await screen.findByTestId("backlog-pipeline-mode-quick");
    expect(screen.queryByTestId("backlog-pipeline-mode-empty-hint")).not.toBeInTheDocument();
  });

  it("G-2: renders a synthetic disabled 'Unknown mode' option when the item's stored slug is unresolvable, and never falls back to Default", async () => {
    mockListPipelineModes(() => Promise.resolve([QUICK_MODE, FULL_MODE]));

    render(
      <BacklogItemForm
        initialValues={{ id: "item-1", title: "Existing item", pipelineMode: "legacy-fast" }}
        onSubmit={jest.fn()}
        onCancel={jest.fn()}
      />
    );

    const unknownOption = await screen.findByTestId("backlog-pipeline-mode-unknown-legacy-fast");
    expect(unknownOption).toHaveAttribute("aria-checked", "true");
    expect(unknownOption).toHaveAttribute("aria-disabled", "true");
    expect(unknownOption).toBeDisabled();
    expect(unknownOption).toHaveTextContent("Unknown mode ('legacy-fast')");

    expect(
      screen.getByText(
        "This item references a pipeline mode that no longer exists or is disabled. Choosing a different mode below will replace it when you save."
      )
    ).toBeInTheDocument();

    // Never silently falls back to Default appearing selected.
    expect(screen.getByTestId("backlog-pipeline-mode-default")).toHaveAttribute(
      "aria-checked",
      "false"
    );
  });

  it("G-1: warns (non-blocking) when the selected mode needs {{repo_path}} but the item's repoPath is empty", async () => {
    mockListPipelineModes(() => Promise.resolve([QUICK_MODE, FULL_MODE]));

    render(
      <BacklogItemForm
        // Editing an existing item (id present) so the pre-existing "repo path
        // required" create-only validation doesn't also block submission —
        // isolates the G-1 behavior under test.
        initialValues={{ id: "item-1", title: "Existing item", repoPath: "" }}
        onSubmit={jest.fn()}
        onCancel={jest.fn()}
      />
    );

    await screen.findByTestId("backlog-pipeline-mode-full");
    fireEvent.click(screen.getByTestId("backlog-pipeline-mode-full"));

    const warning = await screen.findByTestId("backlog-pipeline-mode-repo-path-warning");
    expect(warning).toHaveAttribute("role", "alert");
    expect(warning).toHaveTextContent("⚠ Full SDD mode requires a repository path — add one above.");

    // Non-blocking: selection remains, and Save stays enabled.
    expect(screen.getByTestId("backlog-pipeline-mode-full")).toHaveAttribute("aria-checked", "true");
    expect(screen.getByTestId("backlog-form-submit")).not.toBeDisabled();
  });
});

// project_plans/backlog-sdd-default-pipeline: pre-selecting the seeded "sdd"
// pipeline mode for a brand-new item when the operator has opted in via the
// backlog:sdd-default-pipeline feature flag. This is a default, not a lock —
// the user can still pick any other mode before saving.
describe("BacklogItemForm — sdd default pipeline pre-selection", () => {
  beforeEach(() => {
    jest.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("pre-selects sdd when creating a new item, flag on, and an enabled sdd mode is available", async () => {
    mockUseFeatureFlag.mockReturnValue(true);
    mockListPipelineModes(() => Promise.resolve([QUICK_MODE, SDD_MODE]));

    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    const sddOption = await screen.findByTestId("backlog-pipeline-mode-sdd");
    await waitFor(() => expect(sddOption).toHaveAttribute("aria-checked", "true"));
    expect(screen.getByTestId("backlog-pipeline-mode-default")).toHaveAttribute("aria-checked", "false");
  });

  it("never auto-selects sdd when editing an existing item, even with the flag on", async () => {
    mockUseFeatureFlag.mockReturnValue(true);
    mockListPipelineModes(() => Promise.resolve([QUICK_MODE, SDD_MODE]));

    render(
      <BacklogItemForm
        initialValues={{ id: "item-1", title: "Existing item" }}
        onSubmit={jest.fn()}
        onCancel={jest.fn()}
      />
    );

    await screen.findByTestId("backlog-pipeline-mode-sdd");
    expect(screen.getByTestId("backlog-pipeline-mode-default")).toHaveAttribute("aria-checked", "true");
    expect(screen.getByTestId("backlog-pipeline-mode-sdd")).toHaveAttribute("aria-checked", "false");
  });

  it("does not pre-select sdd when the flag is off", async () => {
    mockUseFeatureFlag.mockReturnValue(false);
    mockListPipelineModes(() => Promise.resolve([QUICK_MODE, SDD_MODE]));

    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    await screen.findByTestId("backlog-pipeline-mode-sdd");
    expect(screen.getByTestId("backlog-pipeline-mode-default")).toHaveAttribute("aria-checked", "true");
    expect(screen.getByTestId("backlog-pipeline-mode-sdd")).toHaveAttribute("aria-checked", "false");
  });

  it("does not pre-select anything when the flag is on but no sdd mode is available", async () => {
    mockUseFeatureFlag.mockReturnValue(true);
    mockListPipelineModes(() => Promise.resolve([QUICK_MODE, FULL_MODE]));

    render(<BacklogItemForm onSubmit={jest.fn()} onCancel={jest.fn()} />);

    await screen.findByTestId("backlog-pipeline-mode-quick");
    expect(screen.getByTestId("backlog-pipeline-mode-default")).toHaveAttribute("aria-checked", "true");
  });

  it("lets the user override the pre-selected sdd default before submitting", async () => {
    mockUseFeatureFlag.mockReturnValue(true);
    mockListPipelineModes(() => Promise.resolve([QUICK_MODE, SDD_MODE]));
    const onSubmit = jest.fn().mockResolvedValue(undefined);

    render(<BacklogItemForm onSubmit={onSubmit} onCancel={jest.fn()} />);

    await waitFor(() =>
      expect(screen.getByTestId("backlog-pipeline-mode-sdd")).toHaveAttribute("aria-checked", "true")
    );

    fireEvent.click(screen.getByTestId("backlog-pipeline-mode-default"));
    expect(screen.getByTestId("backlog-pipeline-mode-default")).toHaveAttribute("aria-checked", "true");

    fireEvent.change(screen.getByTestId("backlog-title-input"), {
      target: { value: "Some title" },
    });
    fireEvent.change(screen.getByTestId("backlog-repo-path-input"), {
      target: { value: "/home/user/project" },
    });
    fireEvent.click(screen.getByTestId("backlog-form-submit"));

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ pipelineMode: "" }))
    );
  });
});

describe("BacklogItemForm — acceptance criteria row identity", () => {
  it("BacklogItemForm_should_PreserveRowIdentity_When_EarlierRowRemoved", () => {
    render(
      <BacklogItemForm
        initialValues={{
          acCriteria: [
            { index: 0, text: "First criterion", status: "pending" },
            { index: 1, text: "Second criterion", status: "pending" },
          ],
        }}
        onSubmit={jest.fn()}
        onCancel={jest.fn()}
      />
    );

    const secondRowInput = screen.getByTestId("backlog-criterion-text-1");
    secondRowInput.focus();

    fireEvent.click(screen.getByTestId("backlog-remove-criterion-0"));

    const survivingInput = screen.getByTestId("backlog-criterion-text-0");
    expect(survivingInput).toBe(secondRowInput);
    expect(survivingInput).toHaveValue("Second criterion");
    expect(document.activeElement).toBe(survivingInput);
  });

  it("strips clientKey from the acCriteria payload on submit", async () => {
    const onSubmit = jest.fn().mockResolvedValue(undefined);
    render(
      <BacklogItemForm
        initialValues={{
          id: "item-1",
          title: "Existing item",
          acCriteria: [{ index: 0, text: "A criterion", status: "pending" }],
        }}
        onSubmit={onSubmit}
        onCancel={jest.fn()}
      />
    );

    fireEvent.click(screen.getByTestId("backlog-form-submit"));

    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    const { acCriteria } = onSubmit.mock.calls[0][0];
    expect(acCriteria).toEqual([{ index: 0, text: "A criterion", status: "pending" }]);
  });
});
