import type { NextConfig } from "next";

const config: NextConfig = {
  reactStrictMode: true,
  output: "standalone",
  turbopack: { root: process.cwd() },
  images: { unoptimized: true },
  async rewrites() {
    return [
      { source: "/api/v1/:path*", destination: `${process.env.API_INTERNAL_URL || "http://127.0.0.1:8083"}/api/v1/:path*` },
    ];
  },
};

export default config;
