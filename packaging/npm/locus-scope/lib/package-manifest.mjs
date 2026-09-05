import { readFile } from 'node:fs/promises';

const SOURCE_VERSION = new URL('../../../../VERSION', import.meta.url);
const PACKAGE_MANIFEST = new URL('../package.json', import.meta.url);
const SEMANTIC_VERSION = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

export async function readPackageManifest() {
  const manifest = await readManifest(PACKAGE_MANIFEST);
  if (manifest.version !== undefined) return manifest;
  if (manifest.private !== true) {
    throw new Error('@sundw/locus-scope package manifest is missing version');
  }

  let version;
  try {
    version = (await readFile(SOURCE_VERSION, 'utf8')).trim();
  } catch (error) {
    throw new Error('read @sundw/locus-scope version from repository VERSION', { cause: error });
  }
  if (!SEMANTIC_VERSION.test(version)) {
    throw new Error(`repository VERSION contains an invalid semantic version: ${JSON.stringify(version)}`);
  }

  return {
    ...manifest,
    version,
    optionalDependencies: Object.fromEntries(
      Object.entries(manifest.optionalDependencies ?? {}).map(([name, range]) => [
        name,
        range === 'workspace:*' ? version : range,
      ]),
    ),
  };
}

export async function readManifest(filename) {
  let source;
  try {
    source = await readFile(filename, 'utf8');
  } catch (error) {
    throw new Error(`read package manifest ${JSON.stringify(String(filename))}: ${error.message}`, {
      cause: error,
    });
  }
  try {
    const manifest = JSON.parse(source);
    if (manifest === null || typeof manifest !== 'object' || Array.isArray(manifest)) {
      throw new Error('expected a JSON object');
    }
    return manifest;
  } catch (error) {
    throw new Error(`parse package manifest ${JSON.stringify(String(filename))}: ${error.message}`, {
      cause: error,
    });
  }
}
