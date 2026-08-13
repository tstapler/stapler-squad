import React from "react";
import type { Preview } from "@storybook/react";
import { withThemeByClassName } from "@storybook/addon-themes";
import { THEME_CLASSES } from "../src/lib/contexts/ThemeContext";
import "../src/app/globals.css";

const preview: Preview = {
  parameters: {
    layout: "centered",
    backgrounds: { disable: true },
  },
  decorators: [
    withThemeByClassName({
      themes: THEME_CLASSES,
      defaultTheme: "matrix",
    }),
  ],
};

export default preview;
