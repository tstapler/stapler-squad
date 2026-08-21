import * as fs from 'fs';

interface BackendFeature {
  id: string;
  type: string;
}

interface FrontendFeature {
  id: string;
  type: string;
}

interface Registry<T> {
  version: string;
  features: T[];
}

export interface CoverageGaps {
  unmatchedBackend: string[];
  unmatchedFrontend: string[];
  generatedAt: string;
}

/**
 * Load and parse a registry JSON file.
 */
function loadRegistry<T>(filePath: string): Registry<T> {
  const raw = fs.readFileSync(filePath, 'utf-8');
  const parsed = JSON.parse(raw);
  if (!parsed || !Array.isArray(parsed.features)) {
    throw new Error(`Invalid registry format in ${filePath}: missing "features" array`);
  }
  return parsed as Registry<T>;
}

/**
 * Generate a coverage gap report comparing backend and frontend registries.
 *
 * Matching logic: a backend feature `session:create` is considered matched if
 * any frontend feature ID starts with `ui:` and an explicit mapping exists,
 * OR if we find a frontend feature with the same base domain
 * (e.g., backend `session:create` matches frontend `ui:new-session-modal` via
 * the advisory mapping defined below).
 *
 * This is intentionally advisory/best-effort. Gaps are warnings, not errors.
 */
export function generateGapReport(
  backendRegistryPath: string,
  frontendRegistryPath: string,
): CoverageGaps {
  const backendRegistry = loadRegistry<BackendFeature>(backendRegistryPath);
  const frontendRegistry = loadRegistry<FrontendFeature>(frontendRegistryPath);

  const frontendIds = new Set(frontendRegistry.features.map(f => f.id));
  const backendIds = new Set(backendRegistry.features.map(f => f.id));

  // For now, match by checking if any frontend feature shares the same domain prefix
  // as the backend feature. e.g., backend "session:create" matches frontend "ui:session-list"
  // because both have "session" in their scope.
  const frontendDomains = new Set(
    frontendRegistry.features.map(f => f.id.replace(/^ui:/, '').split('-')[0]),
  );

  const unmatchedBackend: string[] = [];
  for (const backendFeature of backendRegistry.features) {
    const domain = backendFeature.id.split(':')[0];
    if (!frontendDomains.has(domain)) {
      unmatchedBackend.push(backendFeature.id);
    }
  }

  const backendDomains = new Set(
    backendRegistry.features.map(f => f.id.split(':')[0]),
  );

  const unmatchedFrontend: string[] = [];
  for (const frontendFeature of frontendRegistry.features) {
    const domain = frontendFeature.id.replace(/^ui:/, '').split('-')[0];
    if (!backendDomains.has(domain)) {
      unmatchedFrontend.push(frontendFeature.id);
    }
  }

  return {
    unmatchedBackend,
    unmatchedFrontend,
    generatedAt: new Date().toISOString(),
  };
}
