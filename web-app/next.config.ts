import type { NextConfig } from "next";

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
    // Turbopack configuration for protobuf .js to .ts resolution
    turbo: {
      resolveAlias: {
        // Note: Turbopack handles this differently - we need symlinks or alias in tsconfig
      },
      resolveExtensions: ['.ts', '.tsx', '.js', '.jsx', '.json'],
    },
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

export default nextConfig;
