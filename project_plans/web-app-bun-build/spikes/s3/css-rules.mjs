// CSS rule extraction through Chromium's CSSOM (no postcss in the isolated Bun tree).
// extractRules(page, cssText) -> [{ctx, sel, body}] in source order: one entry per individual selector of every
// style rule, `ctx` the enclosing at-rule chain ("" at top level), `body` the declaration text.
export async function extractRules(page, cssText) {
  return page.evaluate((text) => {
    const sheet = new CSSStyleSheet();
    sheet.replaceSync(text);
    const out = [];
    const splitSelectors = (s) => {
      const parts = [];
      let depth = 0;
      let cur = "";
      for (const ch of s) {
        if (ch === "(" || ch === "[") depth++;
        if (ch === ")" || ch === "]") depth--;
        if (ch === "," && depth === 0) {
          parts.push(cur.trim());
          cur = "";
        } else cur += ch;
      }
      parts.push(cur.trim());
      return parts;
    };
    const walk = (rules, ctx) => {
      for (const r of rules) {
        if (r.selectorText !== undefined && r.style) {
          for (const sel of splitSelectors(r.selectorText)) out.push({ ctx, sel, body: r.style.cssText });
          if (r.cssRules?.length) walk(r.cssRules, ctx + "{" + r.selectorText + "}");
        } else if (r.cssRules) {
          walk(r.cssRules, ctx + "@" + (r.conditionText ?? r.name ?? r.cssText.split("{")[0]).trim() + " ");
        }
      }
    };
    walk(sheet.cssRules, "");
    return out;
  }, cssText);
}
