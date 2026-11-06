import React from "react";
import { render, waitFor } from "@testing-library/react";
import { installJsdomLayoutShims } from "@/lib/files/__tests__/fixtures/jsdomLayout";
import { CodeEditor, type CodeEditorHandle } from "../CodeEditor";

installJsdomLayoutShims();

const SLO_MS = 300;
const FAIL_AT_MS = SLO_MS * 3;

/** About 1 MB / 25,000 lines of source-like text, identical on every call. */
function oneMegabyteFixture(): string {
  const lines: string[] = [];
  for (let i = 1; i <= 25_000; i++) {
    lines.push(`func h${i}(c Ctx) error { return nil } // ${i}`.slice(0, 40).padEnd(40, " "));
  }
  return lines.join("\n");
}

const nextFrame = () => new Promise<void>((r) => requestAnimationFrame(() => r()));

describe("CodeEditor open latency", () => {
  it("codeEditor_should_PaintOneMbFileUnder900ms_When_OpenedInReadMode", async () => {
    const doc = oneMegabyteFixture();
    let handle: CodeEditorHandle | null = null;
    const { container } = render(
      <CodeEditor content="" language="go" path="big.go" mode="Read" onReady={(h) => (handle = h)} />,
    );
    await waitFor(() => expect(handle).not.toBeNull());

    const start = performance.now();
    const state = await handle!.createBufferState({ doc, language: "go" });
    handle!.setBufferState(state);
    await nextFrame();
    const elapsed = performance.now() - start;

    expect(container.querySelector(".cm-line")).not.toBeNull();
    expect(handle!.getText().length).toBe(doc.length);
    // eslint-disable-next-line no-console
    console.info(`[open-latency] 1 MB Read-mode open to first paint: ${elapsed.toFixed(0)} ms (SLO ${SLO_MS} ms, fail at ${FAIL_AT_MS} ms)`);
    expect(elapsed).toBeLessThan(FAIL_AT_MS);
  });

  it("codeEditor_should_FailLoudly_When_FixtureGenerationNotDeterministic", () => {
    const a = oneMegabyteFixture();
    const b = oneMegabyteFixture();
    expect(a).toBe(b);
    expect(a.length).toBeGreaterThan(1_000_000);
    expect(a.length).toBeLessThan(1_100_000);
    expect(a.split("\n")).toHaveLength(25_000);
  });
});
