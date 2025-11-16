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
  },
  // Disable minification for development builds (better debugging)
  ...(isDevelopmentBuild ? {
    compiler: {
      removeConsole: false,
    },
  } : {}),
  webpack: (config, { dev }) => {
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

    // Only optimize CSS chunking in production to prevent preload warnings
    if (!dev && !isDevelopmentBuild) {
      config.optimization = {
        ...config.optimization,
        splitChunks: {
          ...config.optimization?.splitChunks,
          cacheGroups: {
            ...(config.optimization?.splitChunks as any)?.cacheGroups,
            // Combine all CSS into fewer chunks to reduce preload issues
            styles: {
              name: 'styles',
              test: /\.css$/,
              chunks: 'all',
              enforce: true,
              priority: 10,
            },
          },
        },
      };
    }

    return config;
  },
};

export default nextConfig;
