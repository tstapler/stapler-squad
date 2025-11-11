/**
 * Get the API base URL for the application
 *
 * In production/test: Uses the current window location (same origin)
 * In development: Falls back to localhost:8543
 *
 * This ensures tests running on port 8544 connect to the test server,
 * while development and production use appropriate URLs.
 */
export function getApiBaseUrl(): string {
  // In browser environment
  if (typeof window !== 'undefined') {
    // Use the current origin (hostname + port)
    return window.location.origin;
  }

  // Fallback for server-side rendering or development
  return process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8543';
}
