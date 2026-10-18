import { chooseFilesLayout, MOBILE_CONTAINER_MAX, PHONE_SHORT_SIDE_MAX } from "../chooseFilesLayout";

type Row = { containerWidth: number; pointerCoarse: boolean; screenShortSide: number; expected: "mobile" | "desktop" };

const WIDTHS = [390, 767, 768, 844];
const SHORT_SIDES = [390, 600, 820];

describe("chooseFilesLayout", () => {
  it("chooseFilesLayout_should_ReturnMobileForNarrowContainerOrCoarseShortScreen_And_StayDesktopOnTabletWithKeyboard", () => {
    const rows: Row[] = [];
    for (const containerWidth of WIDTHS) {
      for (const pointerCoarse of [true, false]) {
        for (const screenShortSide of SHORT_SIDES) {
          const mobile = containerWidth < 768 || (pointerCoarse && screenShortSide < 600);
          rows.push({ containerWidth, pointerCoarse, screenShortSide, expected: mobile ? "mobile" : "desktop" });
        }
      }
    }
    expect(rows).toHaveLength(24);
    for (const { expected, ...input } of rows) {
      expect({ input, got: chooseFilesLayout(input) }).toEqual({ input, got: expected });
    }
  });

  it("chooseFilesLayout_should_KeepLandscapePhoneMobile_And_TabletDesktop", () => {
    expect(chooseFilesLayout({ containerWidth: 844, pointerCoarse: true, screenShortSide: 390 })).toBe("mobile");
    expect(chooseFilesLayout({ containerWidth: 1180, pointerCoarse: true, screenShortSide: 820 })).toBe("desktop");
    expect(chooseFilesLayout({ containerWidth: 820, pointerCoarse: true, screenShortSide: 820 })).toBe("desktop");
  });

  it("chooseFilesLayout_should_FollowThe768Rule_When_FinePointerAnyScreen", () => {
    expect(chooseFilesLayout({ containerWidth: 767, pointerCoarse: false, screenShortSide: 1080 })).toBe("mobile");
    expect(chooseFilesLayout({ containerWidth: 768, pointerCoarse: false, screenShortSide: 390 })).toBe("desktop");
  });

  it("chooseFilesLayout_should_NotDependOnVisualViewport_So_KeyboardOrRotationCannotFlipIt", () => {
    // The function takes no height: a tablet whose keyboard shrank the viewport to 470 stays desktop.
    expect(chooseFilesLayout.length).toBe(1);
    expect(chooseFilesLayout({ containerWidth: 1180, pointerCoarse: true, screenShortSide: 820 })).toBe("desktop");
    expect(MOBILE_CONTAINER_MAX).toBe(768);
    expect(PHONE_SHORT_SIDE_MAX).toBe(600);
  });
});
