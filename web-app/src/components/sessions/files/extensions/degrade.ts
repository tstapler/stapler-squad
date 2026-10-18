import { EditorView, lineNumbers } from "@codemirror/view";
import type { Extension } from "@codemirror/state";
import { basicSetup, minimalSetup } from "codemirror";
import type { DegradeProfile } from "@/lib/files/degradeProfile";

/** basicSetup minus what costs on a phone: fold gutter, bracket matching, active line, autocompletion, search keymap. */
const lightSetup = (): Extension => [lineNumbers(), minimalSetup];

/** The editor's base feature set for this buffer; a degraded buffer also skips its language parser (see CmEngine). */
export function setupFor(profile: DegradeProfile | undefined): Extension {
  return profile?.degraded ? lightSetup() : basicSetup;
}

/** Marks a degraded buffer so tests and styles can tell. */
export function degradeExtension(profile?: DegradeProfile): Extension {
  return profile?.degraded ? EditorView.contentAttributes.of({ "data-degraded": "true" }) : [];
}
