/**
 * Keeps a renderer's own hidden textarea from competing with the layer's. The renderer keeps working
 * for physical keyboards; it just never raises the soft keyboard or takes a tab stop while the layer owns typing.
 */

interface Saved {
  inputmode: string | null;
  tabindex: string | null;
  holders: number;
}

const saved = new WeakMap<HTMLTextAreaElement, Saved>();

function restore(el: HTMLTextAreaElement, name: string, value: string | null): void {
  if (value === null) el.removeAttribute(name);
  else el.setAttribute(name, value);
}

/**
 * Marks `el` inert (`inputmode="none"`, `tabindex="-1"`), blurs it if focused, and returns the undo.
 * Re-entrant: the original attributes come back when the last holder lets go.
 */
export function suppressHelperTextarea(el: HTMLTextAreaElement | null | undefined): () => void {
  if (!el) return () => undefined;
  let entry = saved.get(el);
  if (!entry) {
    entry = { inputmode: el.getAttribute("inputmode"), tabindex: el.getAttribute("tabindex"), holders: 0 };
    saved.set(el, entry);
  }
  entry.holders++;
  el.setAttribute("inputmode", "none");
  el.setAttribute("tabindex", "-1");
  if (el.ownerDocument.activeElement === el) el.blur();
  let released = false;
  return () => {
    if (released) return;
    released = true;
    const s = saved.get(el);
    if (!s || --s.holders > 0) return;
    saved.delete(el);
    restore(el, "inputmode", s.inputmode);
    restore(el, "tabindex", s.tabindex);
  };
}
