import type { NextConfig } from "next";
import { createVanillaExtractPlugin } from "@vanilla-extract/next-plugin";

const withVanillaExtract = createVanillaExtractPlugin();

const isDevelopmentBuild = process.env.NEXT_BUILD_MODE === 'development';

const nextConfig: NextConfig = {
  output: "export",
  basePath: "",
  trailingSlash: true,
  // Enable source maps in production builds
  productionBrowserSourceMaps: true,
  // Use unminified React for dev tool (better error messages)
  reactStrictMode: true,
  eslint: {
    // Ignore eslint warnings during build (generated files have warnings)
    ignoreDuringBuilds: true,
  },
  experimental: {
    // Optimize package imports to reduce CSS chunking and preload warnings
    optimizePackageImports: ['@/components', '@/lib'],
  },
  // Disable minification for development builds (better debugging)
  ...(isDevelopmentBuild ? {
    compiler: {
      removeConsole: false,
    },
  } : {}),
  webpack: (config, { dev, isServer }) => {
    // Handle .js imports for .ts files (for generated protobuf code)
    config.resolve.extensionAlias = {
      '.js': ['.js', '.ts', '.tsx'],
      '.mjs': ['.mjs', '.mts'],
      '.cjs': ['.cjs', '.cts'],
    };

    // Disable minification for development builds (better error messages)
    if (isDevelopmentBuild) {
      config.optimization = {
        ...config.optimization,
        minimize: false,
      };
    }

    // Redirect webpack's persistent filesystem cache outside the worktree so
    // multiple git worktrees of this repo can share compiled module cache
    // entries instead of each starting cold. The Makefile's web-app/out
    // target sets this to ~/.stapler-squad/nextjs-webpack-cache by default.
    // webpack's cache pack files are content-hashed and written atomically
    // (temp file + rename), so concurrent builds sharing this directory are
    // safe — a race just means last-writer-wins on a pack file, not
    // corruption. Verified with concurrent builds across two worktrees.
    const sharedCacheDir = process.env.NEXTJS_SHARED_CACHE_DIR;
    if (sharedCacheDir && config.cache && typeof config.cache === 'object') {
      config.cache = {
        ...config.cache,
        cacheDirectory: sharedCacheDir,
      };
    }

    // Consolidate CSS chunks to prevent preload warnings
    // This forces all CSS into fewer chunks that load together
    if (!isServer && !dev) {
      const splitChunks = config.optimization?.splitChunks || {};
      config.optimization = {
        ...config.optimization,
        splitChunks: {
          ...splitChunks,
          cacheGroups: {
            ...(typeof splitChunks === 'object' ? splitChunks.cacheGroups : {}),
            // Bundle all CSS modules together
            styles: {
              name: 'styles',
              type: 'css/mini-extract',
              chunks: 'all',
              enforce: true,
              priority: 100,
            },
          },
        },
      };
    }

    return config;
  },
};

export default withVanillaExtract(nextConfig);
