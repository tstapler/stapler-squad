import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { WorkflowRunDialog, workflowTakesInput, MAX_WORKFLOW_ATTACHMENTS } from "./WorkflowRunDialog";

jest.mock("@/lib/contexts/AnalyticsContext", () => ({ useAnalytics: () => ({ track: jest.fn() }) }));
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://test/api" }));

function png(name = "a.png"): File {
  return new File([new Uint8Array([1, 2, 3])], name, { type: "image/png" });
}

function pickFiles(files: File[]) {
  fireEvent.change(screen.getByTestId("workflow-run-file-input"), { target: { files } });
}

function mockUploads(handler: (file: File, n: number) => Partial<Response> & { jsonBody?: unknown }) {
  let n = 0;
  const fetchMock = jest.fn(async (_url: string, init?: RequestInit) => {
    const file = (init?.body as FormData).get("file") as File;
    const r = handler(file, n++);
    return {
      ok: r.ok ?? true,
      status: r.status ?? 200,
      json: async () => r.jsonBody,
    } as Response;
  });
  global.fetch = fetchMock as unknown as typeof fetch;
  return fetchMock;
}

describe("workflowTakesInput", () => {
  it("is true only when command or input_template has {{input}}", () => {
    expect(workflowTakesInput({ command: "run {{input}}", inputTemplate: "" })).toBe(true);
    expect(workflowTakesInput({ command: "run", inputTemplate: "ctx {{input}}" })).toBe(true);
    expect(workflowTakesInput({ command: "run", inputTemplate: "" })).toBe(false);
  });
});

describe("WorkflowRunDialog", () => {
  const originalFetch = global.fetch;
  afterEach(() => {
    global.fetch = originalFetch;
  });

  it("uploads picked images to the workflow route, shows a thumbnail, and submits arg + paths", async () => {
    const fetchMock = mockUploads((_f, n) => ({ jsonBody: { path: `/att/${n}.png`, filename: `${n}.png` } }));
    const onRun = jest.fn().mockResolvedValue(true);
    render(<WorkflowRunDialog workflowName="handy-plan" onRun={onRun} onCancel={jest.fn()} />);

    fireEvent.change(screen.getByTestId("workflow-run-input"), { target: { value: "build a deck" } });
    pickFiles([png("one.png"), png("two.png")]);

    await waitFor(() => {
      const thumbs = screen.getAllByTestId("workflow-run-thumb");
      expect(thumbs).toHaveLength(2);
      thumbs.forEach((t) => expect(t).toHaveAttribute("data-status", "done"));
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[0][0]).toBe("http://test/api/v1/upload-workflow-attachment");

    fireEvent.click(screen.getByTestId("workflow-run-submit"));
    await waitFor(() => expect(onRun).toHaveBeenCalledTimes(1));
    expect(onRun).toHaveBeenCalledWith("build a deck", ["/att/0.png", "/att/1.png"]);
  });

  it("shows the server error for a rejected upload and does not send that image", async () => {
    mockUploads((_f, n) =>
      n === 0 ? { ok: false, status: 415 } : { jsonBody: { path: "/att/ok.png", filename: "ok.png" } }
    );
    const onRun = jest.fn().mockResolvedValue(true);
    render(<WorkflowRunDialog workflowName="w" onRun={onRun} onCancel={jest.fn()} />);

    pickFiles([png("bad.png"), png("good.png")]);
    await waitFor(() => expect(screen.getByText(/Unsupported image type/)).toBeInTheDocument());
    await waitFor(() => expect(screen.getByTestId("workflow-run-submit")).not.toBeDisabled());

    fireEvent.click(screen.getByTestId("workflow-run-submit"));
    await waitFor(() => expect(onRun).toHaveBeenCalledWith("", ["/att/ok.png"]));
  });

  it("reports a network failure", async () => {
    global.fetch = jest.fn().mockRejectedValue(new Error("down")) as unknown as typeof fetch;
    render(<WorkflowRunDialog workflowName="w" onRun={jest.fn()} onCancel={jest.fn()} />);
    pickFiles([png()]);
    await waitFor(() => expect(screen.getByText(/Network error/)).toBeInTheDocument());
  });

  it("removes an image so it is not submitted", async () => {
    mockUploads((_f, n) => ({ jsonBody: { path: `/att/${n}.png`, filename: "x" } }));
    const onRun = jest.fn().mockResolvedValue(true);
    render(<WorkflowRunDialog workflowName="w" onRun={onRun} onCancel={jest.fn()} />);

    pickFiles([png("one.png"), png("two.png")]);
    await waitFor(() => expect(screen.getAllByTestId("workflow-run-thumb")).toHaveLength(2));
    await waitFor(() => expect(screen.getByTestId("workflow-run-submit")).not.toBeDisabled());

    fireEvent.click(screen.getByRole("button", { name: "Remove one.png" }));
    expect(screen.getAllByTestId("workflow-run-thumb")).toHaveLength(1);

    fireEvent.click(screen.getByTestId("workflow-run-submit"));
    await waitFor(() => expect(onRun).toHaveBeenCalledWith("", ["/att/1.png"]));
  });

  it("blocks submit while an upload is in flight", async () => {
    let release: (v: Response) => void = () => {};
    global.fetch = jest.fn(
      () => new Promise<Response>((resolve) => (release = resolve))
    ) as unknown as typeof fetch;
    render(<WorkflowRunDialog workflowName="w" onRun={jest.fn()} onCancel={jest.fn()} />);

    pickFiles([png()]);
    expect(screen.getByTestId("workflow-run-submit")).toBeDisabled();
    release({ ok: true, status: 200, json: async () => ({ path: "/att/z.png" }) } as Response);
    await waitFor(() => expect(screen.getByTestId("workflow-run-submit")).not.toBeDisabled());
  });

  it("enforces the client-side image limit and skips non-images", async () => {
    const fetchMock = mockUploads((_f, n) => ({ jsonBody: { path: `/att/${n}.png`, filename: "x" } }));
    render(<WorkflowRunDialog workflowName="w" onRun={jest.fn()} onCancel={jest.fn()} />);

    const files = Array.from({ length: MAX_WORKFLOW_ATTACHMENTS + 2 }, (_, i) => png(`f${i}.png`));
    pickFiles([...files, new File(["x"], "notes.txt", { type: "text/plain" })]);

    await waitFor(() => expect(screen.getAllByTestId("workflow-run-thumb")).toHaveLength(MAX_WORKFLOW_ATTACHMENTS));
    expect(fetchMock).toHaveBeenCalledTimes(MAX_WORKFLOW_ATTACHMENTS);
    expect(screen.getByTestId("workflow-run-notice")).toHaveTextContent("3 files were skipped");
    expect(screen.getByTestId("workflow-run-add-images")).toBeDisabled();
  });

  it("accepts dropped images", async () => {
    mockUploads(() => ({ jsonBody: { path: "/att/d.png", filename: "d.png" } }));
    render(<WorkflowRunDialog workflowName="w" onRun={jest.fn()} onCancel={jest.fn()} />);
    fireEvent.drop(screen.getByTestId("workflow-run-dropzone"), { dataTransfer: { files: [png("d.png")] } });
    await waitFor(() => expect(screen.getAllByTestId("workflow-run-thumb")).toHaveLength(1));
  });

  it("stays open and re-enables Run when the run fails", async () => {
    const onRun = jest.fn().mockResolvedValue(false);
    render(<WorkflowRunDialog workflowName="w" onRun={onRun} onCancel={jest.fn()} />);
    fireEvent.click(screen.getByTestId("workflow-run-submit"));
    await waitFor(() => expect(onRun).toHaveBeenCalledWith("", []));
    await waitFor(() => expect(screen.getByTestId("workflow-run-submit")).not.toBeDisabled());
  });

  it("calls onCancel", () => {
    const onCancel = jest.fn();
    render(<WorkflowRunDialog workflowName="w" onRun={jest.fn()} onCancel={onCancel} />);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalled();
  });
});
