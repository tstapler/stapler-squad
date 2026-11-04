// All plain style() class names — see windowTabStripStylesMock.js for why not styleMock.js.
module.exports = Object.fromEntries(
  [
    "previewFrame", "previewLeaf", "previewLeafFocused", "previewLeafEmpty", "previewLeafLabel",
    "card", "cardActive", "cardTitle", "cardPreview", "popover", "popoverCard",
    "overlay", "overlayPanel", "overlayGrid", "overlayHint",
  ].map((n) => [n, n])
);
