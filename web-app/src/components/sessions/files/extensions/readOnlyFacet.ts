import { EditorState, type Extension } from "@codemirror/state";

// Story 2.2.1 grows this into the read-only facet (reason, banner state); for now it is the plain switch.
// editable stays true so the browser still allows text selection and copy.
export function readOnlyExtension(readOnly: boolean): Extension {
  return EditorState.readOnly.of(readOnly);
}
