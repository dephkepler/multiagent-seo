import type { NextConfig } from 'next'

const nextConfig: NextConfig = {
  // Static export: every page is a client component talking to the Go API with
  // a per-launch credential the server here could not use anyway, so there is
  // nothing for a Node runtime to render. It also means no second container to
  // keep patched, and an immutable-cacheable bundle — which matters because a
  // Mini App cold-starts in a WebView on mobile data every time it opens.
  output: 'export',
  // Emits out/<route>/index.html, so the proxy resolves a route with no
  // redirect hop.
  trailingSlash: true,
}

export default nextConfig
