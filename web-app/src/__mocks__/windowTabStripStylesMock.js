// The generic styleMock.js returns a callable proxy for every export, so a plain
// style() class name reaches React as a function ("Invalid value for prop
// `className`"). Plain classes here are strings; only the recipes are callable.
const plain = [
  "windowTabStrip", "windowTabList", "windowTabWrapper", "windowTabEditorWrapper",
  "windowTabInput", "windowAddButton", "windowOnboardingHint", "windowOnboardingHintDismiss",
];
const recipes = ["windowTabButton", "windowTabCloseButton"];

module.exports = {
  ...Object.fromEntries(plain.map((n) => [n, n])),
  ...Object.fromEntries(recipes.map((n) => [n, () => n])),
};
