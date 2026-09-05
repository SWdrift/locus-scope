import { readFile, realpath, stat } from 'node:fs/promises';
import { createRequire } from 'node:module';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

const PACKAGE_NAME = /^(?:@[a-z0-9][a-z0-9._~-]*\/)?[a-z0-9][a-z0-9._~-]*$/;
const PACKAGE_VERSION = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;
const SCOPE_MANIFEST_NAMES = new Set(['locus.yaml', 'locus.yml', 'locus.json']);

export async function buildPackageEnvironment(scopeRoot) {
  const canonicalScopeRoot = await realpath(scopeRoot);
  const rootPackageJson = path.join(canonicalScopeRoot, 'package.json');
  const rootManifest = await readOptionalPackageJson(rootPackageJson);
  if (rootManifest === null) {
    return {
      root: { scopeRoot: canonicalScopeRoot, packageRoot: '', dependencies: {} },
      packages: {},
    };
  }

  const rootDependencies = dependencyNames(rootManifest, rootPackageJson);
  const occurrences = new Map();

  const visitLocusPackage = async (record) => {
    const root = await realpath(path.dirname(record.packageJson));
    const cached = occurrences.get(root);
    if (cached) {
      return cached.identity;
    }

    const { name, version } = packageCoordinates(record.manifest, record.packageJson);
    const identity = `npm:${name}@${version}`;
    const entry = await validateLocusEntry(record.manifest, root, record.packageJson);
    const occurrence = { identity, root, entry, dependencies: null };
    occurrences.set(root, occurrence);

    const dependencies = {};
    for (const dependency of dependencyNames(record.manifest, record.packageJson)) {
      const child = await resolveDependency(record.packageJson, dependency);
      if (child !== null && hasLocusMetadata(child.manifest)) {
        dependencies[dependency] = await visitLocusPackage(child);
      }
    }
    occurrence.dependencies = dependencies;
    return identity;
  };

  const directDependencies = {};
  for (const dependency of rootDependencies) {
    const record = await resolveDependency(rootPackageJson, dependency);
    if (record !== null && hasLocusMetadata(record.manifest)) {
      directDependencies[dependency] = await visitLocusPackage(record);
    }
  }

  const byIdentity = new Map();
  for (const occurrence of occurrences.values()) {
    if (occurrence.dependencies === null) {
      throw new Error(`dependency traversal did not complete for ${occurrence.identity}`);
    }
    const existing = byIdentity.get(occurrence.identity);
    if (!existing) {
      byIdentity.set(occurrence.identity, occurrence);
      continue;
    }
    if (
      existing.entry !== occurrence.entry ||
      JSON.stringify(existing.dependencies) !== JSON.stringify(occurrence.dependencies)
    ) {
      throw new Error(
        `conflicting npm package copies for ${occurrence.identity}: locus.entry or resolved Locus dependencies differ`,
      );
    }
    if (occurrence.root < existing.root) {
      byIdentity.set(occurrence.identity, occurrence);
    }
  }

  const packages = {};
  for (const identity of [...byIdentity.keys()].sort()) {
    const occurrence = byIdentity.get(identity);
    packages[identity] = {
      root: occurrence.root,
      entry: occurrence.entry,
      dependencies: occurrence.dependencies,
    };
  }

  return {
    root: {
      scopeRoot: canonicalScopeRoot,
      packageRoot: canonicalScopeRoot,
      dependencies: directDependencies,
    },
    packages,
  };
}

async function resolveDependency(importerPackageJson, dependency) {
  validatePackageName(dependency, `dependency of ${JSON.stringify(importerPackageJson)}`);
  const importerRequire = createRequire(pathToFileURL(importerPackageJson));
  let packageJsonResolutionError;

  try {
    const packageJson = importerRequire.resolve(`${dependency}/package.json`);
    return readResolvedPackage(packageJson);
  } catch (error) {
    packageJsonResolutionError = error;
  }

  let entry;
  try {
    entry = importerRequire.resolve(dependency);
  } catch {
    return null;
  }

  const record = await findPackageFromEntry(entry, dependency, importerPackageJson);
  if (hasLocusMetadata(record.manifest)) {
    throw new Error(
      `Locus package ${JSON.stringify(dependency)} must export "./package.json"; resolving that subpath failed: ${packageJsonResolutionError.message}`,
      { cause: packageJsonResolutionError },
    );
  }
  return record;
}

async function findPackageFromEntry(entry, dependency, importerPackageJson) {
  let current = path.dirname(await realpath(entry));
  while (true) {
    const packageJson = path.join(current, 'package.json');
    const manifest = await readOptionalPackageJson(packageJson);
    if (manifest !== null) {
      validatePackageName(manifest.name, `package manifest ${JSON.stringify(packageJson)}`);
      if (manifest.name === dependency) {
        return { packageJson: await realpath(packageJson), manifest };
      }
    }
    const parent = path.dirname(current);
    if (parent === current) {
      throw new Error(
        `resolved dependency ${JSON.stringify(dependency)} from ${JSON.stringify(importerPackageJson)}, but could not locate its package.json`,
      );
    }
    current = parent;
  }
}

async function readResolvedPackage(packageJson) {
  const canonicalPackageJson = await realpath(packageJson);
  const manifest = await readPackageJson(canonicalPackageJson);
  validatePackageName(manifest.name, `package manifest ${JSON.stringify(canonicalPackageJson)}`);
  return { packageJson: canonicalPackageJson, manifest };
}

async function readOptionalPackageJson(filename) {
  try {
    return await readPackageJson(filename);
  } catch (error) {
    if (error.cause?.code === 'ENOENT' || error.code === 'ENOENT') {
      return null;
    }
    throw error;
  }
}

async function readPackageJson(filename) {
  let source;
  try {
    source = await readFile(filename, 'utf8');
  } catch (error) {
    throw new Error(`read package manifest ${JSON.stringify(filename)}: ${error.message}`, { cause: error });
  }

  let manifest;
  try {
    manifest = JSON.parse(source);
  } catch (error) {
    throw new Error(`parse package manifest ${JSON.stringify(filename)}: ${error.message}`, { cause: error });
  }
  if (manifest === null || typeof manifest !== 'object' || Array.isArray(manifest)) {
    throw new Error(`package manifest ${JSON.stringify(filename)} must contain a JSON object`);
  }
  return manifest;
}

function packageCoordinates(manifest, filename) {
  if (typeof manifest.version !== 'string' || !PACKAGE_VERSION.test(manifest.version)) {
    throw new Error(`package manifest ${JSON.stringify(filename)} has invalid semantic version ${JSON.stringify(manifest.version)}`);
  }
  return { name: manifest.name, version: manifest.version };
}

function dependencyNames(manifest, filename) {
  if (manifest.dependencies === undefined) {
    return [];
  }
  if (
    manifest.dependencies === null ||
    typeof manifest.dependencies !== 'object' ||
    Array.isArray(manifest.dependencies)
  ) {
    throw new Error(`package manifest ${JSON.stringify(filename)} dependencies must be an object`);
  }

  const names = Object.keys(manifest.dependencies).sort();
  for (const name of names) {
    validatePackageName(name, `dependencies in ${JSON.stringify(filename)}`);
    if (typeof manifest.dependencies[name] !== 'string' || manifest.dependencies[name] === '') {
      throw new Error(`dependency ${JSON.stringify(name)} in ${JSON.stringify(filename)} must have a non-empty string specifier`);
    }
  }
  return names;
}

function validatePackageName(name, context) {
  if (typeof name !== 'string' || !PACKAGE_NAME.test(name)) {
    throw new Error(`${context} has invalid npm package name ${JSON.stringify(name)}`);
  }
}

function hasLocusMetadata(manifest) {
  return Object.prototype.hasOwnProperty.call(manifest, 'locus');
}

async function validateLocusEntry(manifest, root, packageJson) {
  if (
    manifest.locus === null ||
    typeof manifest.locus !== 'object' ||
    Array.isArray(manifest.locus) ||
    typeof manifest.locus.entry !== 'string' ||
    manifest.locus.entry === ''
  ) {
    throw new Error(`Locus package ${JSON.stringify(packageJson)} must declare a non-empty locus.entry string`);
  }

  const declared = manifest.locus.entry;
  if (declared.includes('\\') || path.posix.isAbsolute(declared) || /^[A-Za-z]:\//.test(declared)) {
    throw new Error(`locus.entry ${JSON.stringify(declared)} in ${JSON.stringify(packageJson)} must be a relative slash path`);
  }
  const entry = path.posix.normalize(declared.replace(/^\.\//, ''));
  if (entry === '.' || entry === '..' || entry.startsWith('../')) {
    throw new Error(`locus.entry ${JSON.stringify(declared)} in ${JSON.stringify(packageJson)} escapes the package root`);
  }
  if (!SCOPE_MANIFEST_NAMES.has(path.posix.basename(entry))) {
    throw new Error(`locus.entry ${JSON.stringify(declared)} in ${JSON.stringify(packageJson)} is not a Scope manifest`);
  }

  const entryPath = path.join(root, ...entry.split('/'));
  let entryInfo;
  let canonicalEntry;
  try {
    [entryInfo, canonicalEntry] = await Promise.all([stat(entryPath), realpath(entryPath)]);
  } catch (error) {
    throw new Error(`inspect locus.entry ${JSON.stringify(entryPath)}: ${error.message}`, { cause: error });
  }
  if (!entryInfo.isFile() || escapesRoot(root, canonicalEntry)) {
    throw new Error(`locus.entry ${JSON.stringify(declared)} in ${JSON.stringify(packageJson)} must be a file inside the package root`);
  }
  return entry;
}

function escapesRoot(root, candidate) {
  const relative = path.relative(root, candidate);
  return relative === '..' || relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative);
}
