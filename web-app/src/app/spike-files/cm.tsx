"use client";
// THROWAWAY (files-viewer-revamp Phase 0 spike): minimal CodeMirror 6 host shared by the spike variants.
import { useEffect, useRef } from "react";
import { EditorState, type Extension } from "@codemirror/state";
import { EditorView, lineNumbers } from "@codemirror/view";
import { history } from "@codemirror/commands";

/** Autocorrect-hostile content attributes (Story 0.1.5; applied for real in Task 1.2.1a). */
export const hostileAttributes: Extension = EditorView.contentAttributes.of({
  autocorrect: "off",
  autocapitalize: "off",
  spellcheck: "false",
});

/** view.setState replaces every extension, so each per-tab state must carry the shared ones itself. */
export const sharedExtensions: Extension[] = [
  lineNumbers(),
  history(),
  EditorView.theme({ "&": { height: "100%" }, ".cm-scroller": { overflow: "auto" } }),
];

export function genDoc(lines: number): string {
  return Array.from({ length: lines }, (_, i) => `L${i + 1} text`).join("\n");
}

export interface SpikeWindow extends Window {
  __spike?: { views: Record<string, EditorView>; [k: string]: unknown };
}

export function spikeRegistry(): NonNullable<SpikeWindow["__spike"]> {
  const w = window as SpikeWindow;
  w.__spike ??= { views: {} };
  return w.__spike;
}

export function CmView(props: {
  testId: string;
  doc: string;
  readOnly?: boolean;
  hostile?: boolean;
  height: string;
  onReady?: (view: EditorView) => void;
}) {
  const host = useRef<HTMLDivElement>(null);
  const { testId, doc, readOnly, hostile, onReady } = props;
  useEffect(() => {
    if (!host.current) return;
    const extensions: Extension[] = [...sharedExtensions];
    if (readOnly) extensions.push(EditorState.readOnly.of(true), EditorView.editable.of(false));
    if (hostile) extensions.push(hostileAttributes);
    const view = new EditorView({ state: EditorState.create({ doc, extensions }), parent: host.current });
    spikeRegistry().views[testId] = view;
    onReady?.(view);
    return () => {
      view.destroy();
      delete spikeRegistry().views[testId];
    };
  }, [testId, doc, readOnly, hostile, onReady]);
  return <div ref={host} data-testid={testId} style={{ height: props.height, border: "1px solid #888", overflow: "hidden" }} />;
}
