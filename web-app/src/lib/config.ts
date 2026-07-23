import { Code, ConnectError } from "@connectrpc/connect";
import type { Interceptor } from "@connectrpc/connect";

/**
 * Get the API base URL for the application
 *
 * In production/test: Uses the current window location with /api prefix
 * In development: Falls back to localhost:8543/api
 *
 * This ensures tests running on port 8544 connect to the test server,
 * while development and production use appropriate URLs.
 * All ConnectRPC services are mounted under /api/ prefix.
 */
export function getApiBaseUrl(): string {
  // Explicit override always wins, even in the browser (e.g. `next dev`
  // running on its own port against a separately-ported backend).
  if (process.env.NEXT_PUBLIC_API_URL) {
    return process.env.NEXT_PUBLIC_API_URL;
  }

  // In browser environment
  if (typeof window !== 'undefined') {
    // Use the current origin (hostname + port) with /api prefix
    return window.location.origin + '/api';
  }

  // Fallback for server-side rendering or development
  return 'http://localhost:8543/api';
}

/**
 * ConnectRPC interceptor that redirects to /login whenever a call fails
 * with Code.Unauthenticated (HTTP 401).  Attach this to every transport so
 * that expired or wiped sessions surface as a login redirect rather than a
 * silent background failure.
 */
export function createAuthInterceptor(): Interceptor {
  return (next) => async (req) => {
    try {
      return await next(req);
    } catch (err) {
      if (
        typeof window !== 'undefined' &&
        !window.location.pathname.startsWith('/login') &&
        err instanceof ConnectError &&
        err.code === Code.Unauthenticated
      ) {
        window.location.href = '/login';
      }
      throw err;
    }
  };
}
