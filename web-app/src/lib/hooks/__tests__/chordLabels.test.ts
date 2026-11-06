import { chordLabel } from "../chordLabels";
import { FILES_CHORDS } from "../filesChords";

const chord = (id: string) => FILES_CHORDS.find((c) => c.id === id)!;

describe("chordLabels", () => {
  it("chordLabels_should_ResolvePhysicalKeyLabelOrFallBackToUsLabelWithNote", () => {
    const german = new Map([["BracketLeft", "ü"], ["BracketRight", "+"], ["KeyW", "w"]]);
    expect(chordLabel(chord("back"), german)).toEqual({ text: "Alt+Ü", note: undefined });
    expect(chordLabel(chord("closeTab"), german).text).toBe("Alt+W");
    expect(chordLabel(chord("back"), null)).toEqual({ text: "Alt+[", note: "(the key at the US [ position)" });
    expect(chordLabel(chord("forward"), null)).toEqual({ text: "Alt+]", note: "(the key at the US ] position)" });
    expect(chordLabel(chord("closeTab"), null)).toEqual({ text: "Alt+W", note: undefined });
  });

  it("chordLabels_should_UseTheMacModifierNames_When_OnMac", () => {
    expect(chordLabel(chord("quickOpen"), null, true).text).toBe("Cmd+P");
    expect(chordLabel(chord("search"), null, false).text).toBe("Ctrl+Shift+F");
    expect(chordLabel(chord("back"), null, true).text).toBe("Option+[");
    expect(chordLabel(chord("prevHit"), null).text).toBe("Shift+F4");
  });
});
