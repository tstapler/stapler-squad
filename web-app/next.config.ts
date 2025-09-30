import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "export",
  basePath: "",
  eslint: {
    // Ignore eslint warnings during build (generated files have warnings)
    ignoreDuringBuilds: true,
  },
  webpack: (config) => {
    // Handle .js imports for .ts files (for generated protobuf code)
    config.resolve.extensionAlias = {
      '.js': ['.js', '.ts', '.tsx'],
      '.mjs': ['.mjs', '.mts'],
      '.cjs': ['.cjs', '.cts'],
    };
    return config;
  },
};

export default nextConfig;
