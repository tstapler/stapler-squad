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
  // In browser environment
  if (typeof window !== 'undefined') {
    // Use the current origin (hostname + port) with /api prefix
    return window.location.origin + '/api';
  }

  // Fallback for server-side rendering or development
  return process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8543/api';
}
