# UX Research: E2E Baseline Failure Fixes

## 1. Alias Advanced Section: "Advanced" vs "Advanced options" checkbox label

**Component location:** `web-app/src/components/settings/AliasesManager.tsx:582-589`

The current markup wraps the checkbox inside a `<label>` with a sibling `<span>Advanced</span>`. This is a standard implicit-label pattern — the browser computes the accessible name from the label's text content, giving the checkbox an accessible name of **"Advanced"**.

**WCAG verdict:** "Advanced" is a valid, complete accessible name. WCAG 1.3.1 (Info and Relationships) and 4.1.2 (Name, Role, Value) require that form controls have an accessible name, not that the name be maximally descriptive. A single-word toggle label is WCAG-conformant.

**Is "Advanced options" meaningfully better?** Marginally: "Advanced options" is a small clarification when the section header is absent in the screen-reader flow, telling the user the toggle controls a group of options rather than marking the current field as advanced. However, the difference is minor and would not constitute a WCAG failure either way.

**Recommendation:** Update the test to match the component (`"Advanced"`, not `"Advanced options"`). The component label is correct. If the team later decides to rename the toggle, make the change in the component and update the test together — do not change just the test to anticipate a future rename.

---

## 2. Env-var aria-labels: "Environment variable key N" (component) vs "Environment variable N key" (test)

**Component location:** `web-app/src/components/settings/AliasesManager.tsx:606`

```
aria-label="Environment variable key 1"   // component (current)
aria-label="Environment variable 1 key"   // test expectation (incorrect)
```

**Which is clearer for screen readers?**

The component's pattern — `"Environment variable key 1"` — groups the *concept* first and the *instance discriminator* last. This follows standard English noun-phrase order: "type → subtype → ordinal." Screen readers read the full label as one phrase before the user interacts, so users hear "environment variable key one" and know immediately what kind of field this is and that it is the first one.

The test's pattern — `"Environment variable 1 key"` — inserts the ordinal in the middle, interrupting the semantic grouping. It is not idiomatic and would be awkward to hear in a list of multiple inputs: "environment variable one key, environment variable one value, environment variable two key …" vs "environment variable key one, environment variable value one, environment variable key two …" The component's ordering clusters related fields (key/value pair for row N) rather than isolating N as a primary sort key.

**WCAG verdict:** Neither ordering is non-conformant, but `"Environment variable key 1"` is the more natural accessible name. Comparable patterns in native browser UIs and WAI-ARIA authoring practices place the ordinal after the label noun.

**Recommendation:** Update all test expectations to match the component: `"Environment variable key N"` and `"Environment variable value N"`.

---

## 3. Skip-link and focus behavior (route-focus-and-announce)

**Implementation:** `web-app/src/app/RootShell.tsx:27-28`

```tsx
<a href="#main-content" className="skip-link">Skip to main content</a>
<main id="main-content" ref={mainRef} tabIndex={-1} className={mainContent}>
```

`tabIndex={-1}` on `<main>` is the required implementation detail for skip links to work. Without it, `href="#main-content"` scrolls to the element but does not move programmatic focus there in all browsers. With it, clicking the skip link both scrolls and focuses `#main-content`.

**Is the WCAG 2.4.1 (Bypass Blocks) requirement satisfied?** Yes. The pattern — visible/focusable skip link + `<main id="main-content" tabIndex={-1}>` — is the canonical WCAG 2.4.1 implementation, described in Technique G1 and H64. The spec test (line 74-76 of `route-focus-and-announce.spec.ts`) checks that `document.activeElement === document.getElementById('main-content')` after navigation, which validates both the skip-link target's focusability and the `useFocusOnNavigate` hook's post-navigation focus behavior.

**Bun build:** `useFocusOnNavigate` is located in `web-app/src/lib/nav/useFocusOnNavigate.ts` and is imported by `web-app/src/lib/nav/entry.tsx`, which is the navigation facade used by all builds. The `FOCUS_ON_NAVIGATE` policy flag is defined as `true` unconditionally in `web-app/src/lib/nav/policy.ts` — it is not gated on the build system. The Bun build therefore has the same post-navigation focus behavior as the Next build. No separate mechanism is needed.

**Spec test recommendation:** The test is testing correct behavior. If it is failing, the most likely cause is a timing issue (the focus assertion must be `poll`-based, which the test already uses) or the test navigating before the shell is fully mounted. No behavior change is recommended.

---

## 4. Additional accessibility recommendations for the alias settings form

The form is well-structured overall. Specific recommendations:

**Checkbox toggle label visibility.** The "Advanced" toggle has no visible label other than the checkbox text. Consider pairing it with a brief helper text (`aria-describedby`) explaining what "Advanced" reveals, e.g., "Show environment variable and CLI flag settings." This is a usability improvement, not a WCAG compliance issue.

**"Add variable" button text.** The button reads "Add variable" (lowercase "v"). This is correct — the test expected "Add Variable" (title case), which was wrong. "Add variable" is the production text in the component (`AliasesManager.tsx:632`). No change needed.

**Remove button accessible name.** The remove button at line 620 uses `aria-label="Remove environment variable ${ev.key || String(i + 1)}"`. When `ev.key` is populated, this gives a highly specific label: "Remove environment variable DATABASE_URL." When empty, it falls back to "Remove environment variable 1." Both are good patterns. The fallback to ordinal index (not "0") is correct because `i + 1` is used.

**Missing `htmlFor` on the "Environment Variables" label.** The `<label className={labelClass}>Environment Variables</label>` at line 596 is not associated with any `for`/`htmlFor` input (it labels the group, not a single input). This should be a `<fieldset>/<legend>` pattern or an `<div role="group" aria-labelledby="...">` wrapper for the env-var rows. WCAG 1.3.1 requires that groups of related form controls be programmatically grouped. This is a real, low-severity accessibility gap in the current form markup, independent of the test fixes.

**Tags input.** The tag input at line 570 has no visible label — only a placeholder. WCAG 1.3.1 requires that inputs have a programmatic label. Recommend adding `aria-label="Add tag"` to the tag input, or associating a visible label with it.

---

## Summary of test-fix decisions

| Test assertion | Component value | Recommended fix |
|---|---|---|
| Checkbox label `"Advanced options"` | `"Advanced"` | Update test to `"Advanced"` |
| Button `"Add Variable"` | `"Add variable"` | Update test to `"Add variable"` |
| Aria-label `"Environment variable N key"` | `"Environment variable key N"` | Update test to `"Environment variable key N"` |
| Skip-link / focus-on-navigate behavior | Implemented correctly | Fix timing/test-setup issue, not the component |
