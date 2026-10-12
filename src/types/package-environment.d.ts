import type { Environment } from '../packageenv.js';

declare module '../packaging/npm/locus-scope/lib/package-environment.mjs' {
  export function buildPackageEnvironment(scopeRoot: string): Promise<Environment>;
}
