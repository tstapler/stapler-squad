import { parseWebBuildMode, type WebBuildMode } from '../../../scripts/dev-stack/web-build-mode';

/** Which web build this run targets (WEB_BUILD); the one gate for assertions that hold on only one build. */
export function webBuildMode(): WebBuildMode {
  return parseWebBuildMode(process.env);
}

export const isBunBuild = (): boolean => webBuildMode() === 'bun';
