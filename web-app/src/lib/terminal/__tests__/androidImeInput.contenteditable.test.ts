import { attachAndroidImeInput } from "../androidImeInput";

// Mirrors ghostty-web 0.4.0's InputHandler: ignores keyCode 229 and sends composition data only at
// compositionend, on a contenteditable root (no textarea).
function ghosttyLikeRoot() {
  const root = document.createElement("div");
  root.setAttribute("contenteditable", "true");
  document.body.appendChild(root);
  const rendererSent: string[] = [];
  root.addEventListener("compositionend", (e) => {
    const data = (e as CompositionEvent).data;
    if (data) rendererSent.push(data);
  });
  const appSent: string[] = [];
  attachAndroidImeInput(root, (d) => appSent.push(d));
  return { root, rendererSent, appSent };
}

describe("attachAndroidImeInput on a ghostty-web style contenteditable root", () => {
  it("streams composition updates and does not let the renderer resend at compositionend", () => {
    const { root, rendererSent, appSent } = ghosttyLikeRoot();
    root.dispatchEvent(new KeyboardEvent("keydown", { key: "Unidentified", keyCode: 229, bubbles: true }));
    root.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true }));
    root.dispatchEvent(new CompositionEvent("compositionupdate", { data: "h", bubbles: true }));
    root.dispatchEvent(new CompositionEvent("compositionupdate", { data: "hi", bubbles: true }));
    expect(appSent.join("")).toBe("hi");
    root.appendChild(document.createTextNode("hi"));
    root.dispatchEvent(new CompositionEvent("compositionend", { data: "hi", bubbles: true }));
    expect(appSent.join("")).toBe("hi");
    expect(rendererSent).toEqual([]);
    expect(root.textContent).toBe("");
  });

  it("sends non-composing insertText and cancels the DOM edit", () => {
    const { root, appSent } = ghosttyLikeRoot();
    const ev = new InputEvent("beforeinput", { data: "x", inputType: "insertText", bubbles: true, cancelable: true });
    root.dispatchEvent(ev);
    expect(appSent).toEqual(["x"]);
    expect(ev.defaultPrevented).toBe(true);
  });
});
