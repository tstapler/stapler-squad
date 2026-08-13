import * as fs from 'fs';
import * as path from 'path';

// Excluded file patterns (generated code, tests, stories)
const EXCLUDED_PATTERNS = [
  /_pb\.ts$/,
  /\.pb\.ts$/,
  /\.test\.tsx?$/,
  /\.spec\.tsx?$/,
  /\.stories\.tsx?$/,
];

const EXCLUDED_DIRS = ['__tests__', 'node_modules', '.next', 'dist', 'gen'];

// Marker comment pattern — captures everything after the colon to support multiple IDs on one line
// e.g. "// +feature: ui:session-list" or "// +feature: session-list session-search"
const FEATURE_MARKER_RE = /\/\/\s*\+feature:\s*(.+)/;

export interface FrontendFeature {
  id: string;
  type: 'frontend';
  frontend: {
    component: string;
    path: string;
    markerLine: number;
  };
  tested: boolean;
  testIds: string[];
  lastModified: string;
}

/**
 * Check if a file should be excluded from scanning.
 */
function isExcluded(filePath: string): boolean {
  const basename = path.basename(filePath);
  const parts = filePath.split(path.sep);

  for (const dir of EXCLUDED_DIRS) {
    if (parts.includes(dir)) return true;
  }

  for (const pattern of EXCLUDED_PATTERNS) {
    if (pattern.test(basename)) return true;
  }

  return false;
}

/**
 * Extract component name from default export or filename.
 */
function extractComponentName(content: string, filePath: string): string {
  // Try to find default export: "export default function Foo" or "export default class Foo"
  const defaultExportMatch = content.match(/export\s+default\s+(?:function|class)\s+(\w+)/);
  if (defaultExportMatch) {
    return defaultExportMatch[1];
  }

  // Try "const Foo = ... export default Foo"
  const namedExportMatch = content.match(/export\s+default\s+(\w+)/);
  if (namedExportMatch) {
    return namedExportMatch[1];
  }

  // Fall back to filename without extension
  const basename = path.basename(filePath, path.extname(filePath));
  // Remove common suffixes
  return basename.replace(/\.(module|page|component)$/, '');
}

/**
 * Scan a single file for the // +feature: marker in the first 10 lines.
 * Returns a FrontendFeature if found, null otherwise.
 */
export function scanFile(filePath: string, rootDir: string): FrontendFeature | null {
  if (isExcluded(filePath)) return null;

  const ext = path.extname(filePath);
  if (!['.ts', '.tsx', '.js', '.jsx'].includes(ext)) return null;

  let content: string;
  try {
    content = fs.readFileSync(filePath, 'utf-8');
  } catch {
    return null;
  }

  const lines = content.split('\n');
  const checkLines = lines.slice(0, 10); // Only first 10 lines

  for (let i = 0; i < checkLines.length; i++) {
    const match = checkLines[i].match(FEATURE_MARKER_RE);
    if (match) {
      // Use the first whitespace-delimited token as the canonical feature ID.
      // The marker may list multiple IDs: "// +feature: session-list session-search"
      const rawIds = match[1].trim().split(/\s+/);
      const featureId = rawIds[0];
      const componentName = extractComponentName(content, filePath);
      const relativePath = path.relative(rootDir, filePath);

      let mtime: string;
      try {
        const stat = fs.statSync(filePath);
        mtime = stat.mtime.toISOString();
      } catch {
        mtime = new Date().toISOString();
      }

      return {
        id: featureId,
        type: 'frontend',
        frontend: {
          component: componentName,
          path: relativePath,
          markerLine: i + 1,
        },
        tested: false,
        testIds: [],
        lastModified: mtime,
      };
    }
  }

  return null;
}

/**
 * Recursively walk a directory and collect all source file paths.
 */
function walkDir(dir: string): string[] {
  const results: string[] = [];

  let entries: fs.Dirent[];
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true });
  } catch {
    return results;
  }

  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);

    if (entry.isDirectory()) {
      if (!EXCLUDED_DIRS.includes(entry.name)) {
        results.push(...walkDir(fullPath));
      }
    } else if (entry.isFile()) {
      results.push(fullPath);
    }
  }

  return results;
}

/**
 * Scan a directory for all frontend feature-marked components.
 * Returns an array of FrontendFeature, one per marked file.
 * Only the first // +feature: marker per file is used.
 *
 * @param srcDir   Absolute path to the source directory to scan (e.g. /project/web-app/src).
 * @param rootDir  Absolute path to use as the base for relative paths in the registry.
 *                 Defaults to the srcDir parent's parent (project root when srcDir = web-app/src).
 */
export function scanComponents(srcDir: string, rootDir?: string): FrontendFeature[] {
  const resolvedRoot = rootDir ?? path.resolve(srcDir, '../..');
  const allFiles = walkDir(srcDir);
  const features: FrontendFeature[] = [];
  const seenIds = new Set<string>();

  for (const file of allFiles) {
    const feature = scanFile(file, resolvedRoot);
    if (feature) {
      if (seenIds.has(feature.id)) {
        console.warn(`Warning: duplicate feature ID "${feature.id}" in ${file}, skipping`);
        continue;
      }
      seenIds.add(feature.id);
      features.push(feature);
    }
  }

  return features;
}
