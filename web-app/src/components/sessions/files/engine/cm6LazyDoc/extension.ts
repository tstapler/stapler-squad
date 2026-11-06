import type { Extension } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { LINE_HEIGHT, gapTableField, initialSegs, type Seg } from "./gapTable";
import { absoluteLineNumbers } from "./gutter";
import { placeholderGuard } from "./guards";
import { placeholderField } from "./placeholderWidget";
import { demandPlugin, demandSink, type DemandSink } from "./scrollAnchor";

/** Everything a gap-line buffer's EditorState carries; the document itself is `initialDocFor(segs)`. */
export function gapLineExtension(sink: DemandSink, segs: Seg[]): Extension {
  return [
    initialSegs.of(segs),
    gapTableField,
    placeholderField,
    absoluteLineNumbers(),
    placeholderGuard,
    demandSink.of(sink),
    demandPlugin,
    // Fixed line height keeps the placeholder's estimated height exact and scroll anchoring stable.
    EditorView.theme({ ".cm-line": { height: `${LINE_HEIGHT}px`, lineHeight: `${LINE_HEIGHT}px` } }),
  ];
}
