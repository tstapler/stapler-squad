"use client";

import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef } from "react";
import type { EditorView } from "@codemirror/view";
import type { EditorState, Compartment, Extension, StateEffect } from "@codemirror/state";
import { vars } from "@/styles/theme.css";
import type { GutterMarkType } from "@/lib/utils/parseDiff";
import { readOnlyExtension } from "./files/extensions/readOnlyFacet";
import { degradeExtension } from "./files/extensions/degrade";
import { searchHighlightExtension } from "./files/extensions/searchHighlight";
import { vimExtension } from "./files/extensions/vim";
import { mergeExtension } from "./files/extensions/merge";
import {
  codeEditor, gutterMarkerAdd, gutterMarkerDelete, gutterMarkerModify,
} from "./CodeEditor.css";

export type EditorMode = "Read" | "Edit";

export interface BufferInit {
  doc: string;
  language?: string;
  gutterMarks?: Map<number, GutterMarkType>;
}

export interface CodeEditorHandle {
  createBufferState(init: BufferInit): Promise<EditorState>;
  getState(): EditorState;
  setBufferState(state: EditorState): void;
  getText(): string;
  getCursor(): { line: number; col: number };
  /** Inserts at the cursor like typing; false (and no change) while read-only. */
  insertText(text: string): boolean;
  gotoLine(line: number): void;
  scrollSnapshot(): StateEffect<unknown>;
}

export interface CodeEditorProps {
  content: string;
  language: string;
  path?: string;
  mode?: EditorMode;
  wrapLines?: boolean;
  gutterMarks?: Map<number, GutterMarkType>;
  onReady?: (handle: CodeEditorHandle) => void;
}

const GUTTER_MARKER_CLASS: Record<GutterMarkType, string> = {
  add: gutterMarkerAdd,
  delete: gutterMarkerDelete,
  modify: gutterMarkerModify,
};

interface Compartments {
  readOnly: Compartment;
  wrap: Compartment;
  meta: Compartment;
}

export const CodeEditor = forwardRef<CodeEditorHandle, CodeEditorProps>(function CodeEditor(props, ref) {
  const { content, language, path, mode = "Read", wrapLines, gutterMarks, onReady } = props;
  const hostRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const handleRef = useRef<CodeEditorHandle | null>(null);
  const compRef = useRef<Compartments | null>(null);
  const live = useRef({ mode, wrapLines, path, content, language, gutterMarks });
  live.current = { mode, wrapLines, path, content, language, gutterMarks };
  const deferred = useRef<Array<() => void>>([]);

  // The engine mounts asynchronously, so callers get a stable delegate that targets the live handle.
  const delegate = useMemo(() => new Proxy({} as CodeEditorHandle, {
    get: (_, key: keyof CodeEditorHandle) => (...args: unknown[]) =>
      (handleRef.current?.[key] as (...a: unknown[]) => unknown)(...args),
  }), []);
  useImperativeHandle(ref, () => delegate, [delegate]);

  // Mount the view once; later prop changes go through reconfigure/setState, never a rebuild.
  useEffect(() => {
    let cancelled = false;
    let view: EditorView | null = null;
    (async () => {
      if (!hostRef.current) return;
      const { EditorView } = await import("@codemirror/view");
      const state = await import("@codemirror/state");
      const [{ basicSetup }, { HighlightStyle, syntaxHighlighting }, { tags }] = await Promise.all([
        import("codemirror"), import("@codemirror/language"), import("@lezer/highlight"),
      ]);
      if (cancelled || !hostRef.current) return;
      const { Compartment, EditorState } = state;
      const comp: Compartments = { readOnly: new Compartment(), wrap: new Compartment(), meta: new Compartment() };

      const readOnlyConf = (m: EditorMode) => [
        readOnlyExtension(m === "Read"),
        EditorView.contentAttributes.of({ "aria-readonly": String(m === "Read") }),
      ];
      const wrapConf = (w?: boolean) => (w ? [EditorView.lineWrapping] : []);
      const metaConf = (p?: string) => [
        EditorView.contentAttributes.of({
          "aria-label": `File: ${p ?? ""}`,
          autocorrect: "off",
          autocapitalize: "off",
          spellcheck: "false",
        }),
      ];
      const theme = EditorView.theme({
        "&": { color: vars.color.terminalForeground, backgroundColor: vars.color.terminalBackground, height: "100%" },
        ".cm-content": { caretColor: vars.color.terminalCursor },
        ".cm-gutters": {
          backgroundColor: vars.color.terminalBackground,
          color: vars.color.terminalTextMuted,
          borderRight: `1px solid ${vars.color.terminalBorder}`,
        },
      }, { dark: true });
      const highlight = syntaxHighlighting(HighlightStyle.define([
        { tag: [tags.keyword, tags.controlKeyword, tags.operatorKeyword, tags.modifier], color: vars.color.primary },
        { tag: [tags.string, tags.special(tags.string)], color: vars.color.success },
        { tag: [tags.comment, tags.lineComment, tags.blockComment], color: vars.color.terminalTextMuted, fontStyle: "italic" },
        { tag: [tags.number, tags.bool, tags.null], color: vars.color.warning },
      ]));

      const create = async (init: BufferInit): Promise<EditorState> => {
        const { gutter, GutterMarker } = await import("@codemirror/view");
        const lang = await loadLang(init.language ?? "text");
        const marks = init.gutterMarks;
        class Mark extends GutterMarker {
          constructor(private kind: GutterMarkType) { super(); }
          eq(o: Mark) { return o.kind === this.kind; }
          toDOM() {
            const el = document.createElement("div");
            el.className = GUTTER_MARKER_CLASS[this.kind];
            return el;
          }
        }
        const changeGutter = gutter({
          class: "cm-changeGutter",
          lineMarker(v, block) {
            const kind = marks?.get(v.state.doc.lineAt(block.from).number);
            return kind ? new Mark(kind) : null;
          },
        });
        const cur = live.current;
        // setState drops every extension, so each buffer state carries the full set itself.
        return EditorState.create({
          doc: init.doc,
          extensions: [
            basicSetup, theme, highlight, changeGutter,
            comp.readOnly.of(readOnlyConf(cur.mode)),
            comp.wrap.of(wrapConf(cur.wrapLines)),
            comp.meta.of(metaConf(cur.path)),
            degradeExtension(), searchHighlightExtension(), vimExtension(), mergeExtension(),
            lang ?? [],
          ],
        });
      };

      const initial = await create({ doc: live.current.content, language: live.current.language, gutterMarks: live.current.gutterMarks });
      if (cancelled || !hostRef.current) return;
      view = new EditorView({ state: initial, parent: hostRef.current });
      viewRef.current = view;
      compRef.current = comp;
      const v = view;

      const whenIdle = (fn: () => void) => {
        if (!v.compositionStarted) return fn();
        deferred.current.push(fn);
      };
      const flush = () => setTimeout(() => {
        if (v.compositionStarted) return;
        deferred.current.splice(0).forEach((fn) => fn());
      }, 0);
      v.contentDOM.addEventListener("compositionend", flush);

      const syncConfig = () => whenIdle(() => v.dispatch({ effects: [
        comp.readOnly.reconfigure(readOnlyConf(live.current.mode)),
        comp.wrap.reconfigure(wrapConf(live.current.wrapLines)),
        comp.meta.reconfigure(metaConf(live.current.path)),
      ] }));
      (v as EditorView & { __sync?: () => void }).__sync = syncConfig;
      (v as EditorView & { __create?: typeof create }).__create = create;

      handleRef.current = {
        createBufferState: create,
        getState: () => v.state,
        setBufferState: (s) => whenIdle(() => { v.setState(s); syncConfig(); }),
        getText: () => v.state.doc.toString(),
        getCursor: () => {
          const head = v.state.selection.main.head;
          const line = v.state.doc.lineAt(head);
          return { line: line.number, col: head - line.from };
        },
        insertText: (text) => {
          if (v.state.readOnly || v.compositionStarted) return false;
          v.dispatch(v.state.replaceSelection(text), { userEvent: "input.type", scrollIntoView: true });
          return true;
        },
        gotoLine: (n) => {
          const doc = v.state.doc;
          const line = doc.line(Math.min(Math.max(1, Math.floor(n)), doc.lines));
          v.dispatch({ selection: { anchor: line.from }, effects: EditorView.scrollIntoView(line.from, { y: "center" }) });
        },
        scrollSnapshot: () => v.scrollSnapshot() as StateEffect<unknown>,
      };
      onReady?.(handleRef.current);
    })();
    return () => {
      cancelled = true;
      view?.destroy();
      viewRef.current = null;
      compRef.current = null;
      handleRef.current = null;
    };
    // onReady is read once at mount; later prop changes are handled by the effects below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // mode / wrap / path: Compartment reconfigure on the live view.
  useEffect(() => {
    (viewRef.current as (EditorView & { __sync?: () => void }) | null)?.__sync?.();
  }, [mode, wrapLines, path]);

  // A read-only buffer follows its source; an editable one never has its doc replaced from props.
  const seeded = useRef({ content, language, gutterMarks });
  useEffect(() => {
    const prev = seeded.current;
    seeded.current = { content, language, gutterMarks };
    const view = viewRef.current as (EditorView & { __create?: (i: BufferInit) => Promise<EditorState> }) | null;
    if (!view || mode === "Edit") return;
    if (prev.content === content && prev.language === language && prev.gutterMarks === gutterMarks) return;
    void view.__create?.({ doc: content, language, gutterMarks }).then((s) => handleRef.current?.setBufferState(s));
  }, [content, language, gutterMarks, mode]);

  return <div ref={hostRef} className={codeEditor} />;
});

async function loadLang(lang: string): Promise<Extension | null> {
  try {
    const { loadCodemirrorLang } = await import("./files/engine/languages");
    return await loadCodemirrorLang(lang);
  } catch {
    return null;
  }
}
