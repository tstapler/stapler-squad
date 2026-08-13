import type { StorybookConfig } from "@storybook/react-webpack5";
import path from "path";

// NOTE: @storybook/nextjs is incompatible with Next.js 15 due to the bundled
// webpack inside next/dist/compiled/webpack/bundle5.js conflicting with
// Storybook's child compiler hook system ("Cannot read properties of undefined
// reading 'tap'"). Using @storybook/react-webpack5 instead, which uses the
// standalone webpack5 and does not invoke Next.js webpack internals.
// Stories themselves do not require Next.js-specific rendering (no <Image>,
// <Link router>, or Server Components).

const config: StorybookConfig = {
  stories: ["../src/**/*.stories.@(ts|tsx)"],
  addons: [
    "@storybook/addon-essentials",
    "@storybook/addon-themes",
    "@storybook/addon-a11y",
    "@chromatic-com/storybook",
  ],
  framework: {
    name: "@storybook/react-webpack5",
    options: {
      builder: {
        useSWC: true,
      },
    },
  },
  staticDirs: ["../public"],
  webpackFinal: async (config) => {
    config.module = config.module ?? {};
    config.module.rules = config.module.rules ?? [];

    // TypeScript + JSX: use babel-loader (already installed via @storybook/nextjs)
    config.module.rules.push({
      test: /\.(ts|tsx)$/,
      exclude: /node_modules/,
      use: [
        {
          loader: require.resolve("babel-loader"),
          options: {
            presets: [
              ["@babel/preset-env", { targets: { chrome: 100 } }],
              ["@babel/preset-react", { runtime: "automatic" }],
              "@babel/preset-typescript",
            ],
          },
        },
      ],
    });

    // vanilla-extract .css.ts files: mock them out in Storybook to avoid
    // running the VanillaExtractPlugin child compiler. Empty proxy objects
    // mean className references become empty strings — components render
    // correctly for snapshot/a11y testing even without styles.
    config.module.rules.push({
      test: /\.css\.ts$/,
      use: [
        {
          loader: path.resolve(__dirname, "./css-ts-mock-loader.js"),
        },
      ],
    });

    // Handle TypeScript path aliases (@/ → src/)
    config.resolve = config.resolve ?? {};
    config.resolve.alias = {
      ...config.resolve.alias,
      "@": path.resolve(__dirname, "../src"),
    };

    return config;
  },
};

export default config;
