import { readFileSync } from "fs";
import { join } from "path";
import { createInputElement, TEXTAREA_TEST_ID } from "./inputElement";
import { attributesFor, TEXTAREA_ATTRIBUTES } from "./policy";

describe("createInputElement", () => {
  it("createInputElement_should_SetAutocapitalizeNoneAndOmitSpellcheck_When_Created", () => {
    const el = createInputElement(document);
    expect(el.tagName).toBe("TEXTAREA");
    expect(el.getAttribute("autocapitalize")).toBe("none");
    expect(el.getAttribute("autocomplete")).toBe("off");
    expect(el.getAttribute("enterkeyhint")).toBe("send");
    expect(el.getAttribute("spellcheck")).toBeNull();
    expect(el.getAttribute("autocorrect")).toBeNull();
    expect(el.getAttribute("role")).toBe("textbox");
    expect(el.getAttribute("aria-multiline")).toBe("true");
    expect(el.getAttribute("aria-label")).toBe("Terminal input");
    expect(el.getAttribute("data-testid")).toBe("mobile-input-textarea");
    expect(TEXTAREA_TEST_ID).toBe("mobile-input-textarea");
  });

  it("createInputElement_should_BeFocusableAndScreenReaderVisible_When_Created", () => {
    const el = createInputElement(document);
    expect(el.className).toContain("textarea");
    expect(el.style.display).toBe("");
    expect(el.hasAttribute("aria-hidden")).toBe(false);
    expect(el.hasAttribute("hidden")).toBe(false);
    expect(el.tabIndex).toBe(0);
  });

  it("createInputElement_should_LinkStatusRegion_When_DescribedByGiven", () => {
    expect(createInputElement(document, { describedBy: "status-1" }).getAttribute("aria-describedby")).toBe("status-1");
    expect(createInputElement(document).hasAttribute("aria-describedby")).toBe(false);
  });
});

describe("attributesFor", () => {
  it("attributesFor_should_AddAutocorrectOffOnlyWhenEchoOff_When_PolicyEvaluated", () => {
    const normal = attributesFor({ echoOff: false });
    expect(normal).toEqual({ ...TEXTAREA_ATTRIBUTES });
    expect("autocorrect" in normal).toBe(false);
    expect("spellcheck" in normal).toBe(false);
    const secret = attributesFor({ echoOff: true });
    expect(secret).toMatchObject({ autocorrect: "off", spellcheck: "false", autocomplete: "off" });
  });

  it("attributes_should_OmitAutocorrectOff_When_NoEchoOffSignal", () => {
    const el = createInputElement(document);
    expect(el.getAttribute("autocorrect")).toBeNull();
    expect(el.getAttribute("autocomplete")).toBe("off");
  });
});

describe("mobileInput.css.ts", () => {
  const source = readFileSync(join(__dirname, "mobileInput.css.ts"), "utf8");

  it("styles_should_UseThemeTokensAndNoHexLiterals_When_Linted", () => {
    expect(source).not.toMatch(/#[0-9a-fA-F]{3,8}\b/);
    expect(source).not.toMatch(/\brgba?\(/);
  });

  it("styles_should_KeepElementFocusableAndInteractionFree_When_Read", () => {
    expect(source).toMatch(/opacity:\s*0/);
    expect(source).toMatch(/pointerEvents:\s*"none"/);
    expect(source).not.toMatch(/display:\s*"none"/);
    expect(source).not.toMatch(/visibility:\s*"hidden"/);
  });
});
