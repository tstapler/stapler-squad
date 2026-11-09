import * as css from "./mobileInput.css";
import { attributesFor } from "./policy";

export const TEXTAREA_TEST_ID = "mobile-input-textarea";

export interface InputElementOptions {
  /** Id of the polite status region (plan Story 1.4.4); omitted until the chrome exists. */
  describedBy?: string;
}

/** The hidden textarea the soft keyboard types into. Not attached to the DOM here. */
export function createInputElement(doc: Document, opts: InputElementOptions = {}): HTMLTextAreaElement {
  const el = doc.createElement("textarea");
  el.className = String(css.textarea);
  for (const [name, value] of Object.entries(attributesFor({ echoOff: false }))) el.setAttribute(name, value);
  el.setAttribute("role", "textbox");
  el.setAttribute("aria-multiline", "true");
  el.setAttribute("aria-label", "Terminal input");
  el.setAttribute("data-testid", TEXTAREA_TEST_ID);
  el.rows = 1;
  if (opts.describedBy) el.setAttribute("aria-describedby", opts.describedBy);
  return el;
}
