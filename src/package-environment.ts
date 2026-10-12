import { readFile, realpath, stat } from 'node:fs/promises';
import { createRequire } from 'node:module';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

const PACKAGE_NAME = /^(?:@[a-z0-9][a-z0-9._~-]*\/)?[a-z0-9][a-z0-9._~-]*$/;
const PACKAGE_VERSION = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;
const SCOPE_MANIFEST_NAMES = new Set(['locus.yaml', 'locus.yml', 'locus.json']);

type PackageManifest = { name?: unknown; version?: unknown; dependencies?: unknown; locus?: unknown; [key: string]: unknown };
type PackageRecord = { packageJson: string; manifest: PackageManifest };
type Occurrence = { identity: string; root: string; entry: string; dependencies: Record<string, string> | null };
export type PackageDescriptor = { root: string; entry: string; dependencies: Record<string, string> };
export type PackageEnvironment = { rootScope: string; rootDependencies: Record<string, string>; packages: Record<string, PackageDescriptor> };

export async function buildPackageEnvironment(scopeRoot: string): Promise<PackageEnvironment> {
  const canonicalScopeRoot = await realpath(scopeRoot);
  const rootPackageJson = path.join(canonicalScopeRoot, 'package.json');
  const rootManifest = await readOptionalPackageJson(rootPackageJson);
  if (rootManifest === null) return { rootScope: canonicalScopeRoot, rootDependencies: {}, packages: {} };
  const rootDependencies = dependencyNames(rootManifest, rootPackageJson);
  const occurrences = new Map<string, Occurrence>();
  const visitLocusPackage = async (record: PackageRecord): Promise<string> => {
    const root = await realpath(path.dirname(record.packageJson));
    const cached = occurrences.get(root);
    if (cached) return cached.identity;
    const { name, version } = packageCoordinates(record.manifest, record.packageJson);
    const identity = `npm:${name}@${version}`;
    const entry = await validateLocusEntry(record.manifest, root, record.packageJson);
    const occurrence: Occurrence = { identity, root, entry, dependencies: null };
    occurrences.set(root, occurrence);
    const dependencies: Record<string, string> = {};
    for (const dependency of dependencyNames(record.manifest, record.packageJson)) {
      const child = await resolveDependency(record.packageJson, dependency);
      if (child !== null && hasLocusMetadata(child.manifest)) dependencies[dependency] = await visitLocusPackage(child);
    }
    occurrence.dependencies = dependencies;
    return identity;
  };
  const directDependencies: Record<string, string> = {};
  for (const dependency of rootDependencies) {
    const record = await resolveDependency(rootPackageJson, dependency);
    if (record !== null && hasLocusMetadata(record.manifest)) directDependencies[dependency] = await visitLocusPackage(record);
  }
  const byIdentity = new Map<string, Occurrence>();
  for (const occurrence of occurrences.values()) {
    if (occurrence.dependencies === null) throw new Error(`dependency traversal did not complete for ${occurrence.identity}`);
    const existing = byIdentity.get(occurrence.identity);
    if (!existing) byIdentity.set(occurrence.identity, occurrence);
    else if (existing.entry !== occurrence.entry || JSON.stringify(existing.dependencies) !== JSON.stringify(occurrence.dependencies)) throw new Error(`conflicting npm package copies for ${occurrence.identity}: locus.entry or resolved Locus dependencies differ`);
    else if (occurrence.root < existing.root) byIdentity.set(occurrence.identity, occurrence);
  }
  const packages: Record<string, PackageDescriptor> = {};
  for (const identity of [...byIdentity.keys()].sort()) { const occurrence = byIdentity.get(identity)!; packages[identity] = { root: occurrence.root, entry: occurrence.entry, dependencies: occurrence.dependencies! }; }
  return { rootScope: canonicalScopeRoot, rootDependencies: directDependencies, packages };
}

async function resolveDependency(importerPackageJson: string, dependency: string): Promise<PackageRecord | null> {
  validatePackageName(dependency, `dependency of ${JSON.stringify(importerPackageJson)}`);
  const importerRequire = createRequire(pathToFileURL(importerPackageJson));
  let packageJsonResolutionError: unknown;
  try { return readResolvedPackage(importerRequire.resolve(`${dependency}/package.json`)); } catch (error) { packageJsonResolutionError = error; }
  let entry: string;
  try { entry = importerRequire.resolve(dependency); } catch { return null; }
  const record = await findPackageFromEntry(entry, dependency, importerPackageJson);
  if (hasLocusMetadata(record.manifest)) throw new Error(`Locus package ${JSON.stringify(dependency)} must export "./package.json"; resolving that subpath failed: ${errorMessage(packageJsonResolutionError)}`, { cause: packageJsonResolutionError });
  return record;
}

async function findPackageFromEntry(entry: string, dependency: string, importerPackageJson: string): Promise<PackageRecord> {
  let current = path.dirname(await realpath(entry));
  while (true) {
    const packageJson = path.join(current, 'package.json');
    const manifest = await readOptionalPackageJson(packageJson);
    if (manifest !== null) { validatePackageName(manifest.name, `package manifest ${JSON.stringify(packageJson)}`); if (manifest.name === dependency) return { packageJson: await realpath(packageJson), manifest }; }
    const parent = path.dirname(current);
    if (parent === current) throw new Error(`resolved dependency ${JSON.stringify(dependency)} from ${JSON.stringify(importerPackageJson)}, but could not locate its package.json`);
    current = parent;
  }
}
async function readResolvedPackage(packageJson: string): Promise<PackageRecord> { const canonical = await realpath(packageJson); const manifest = await readPackageJson(canonical); validatePackageName(manifest.name, `package manifest ${JSON.stringify(canonical)}`); return { packageJson: canonical, manifest }; }
async function readOptionalPackageJson(filename: string): Promise<PackageManifest | null> { try { return await readPackageJson(filename); } catch (error) { if (errorCode(error) === 'ENOENT') return null; throw error; } }
async function readPackageJson(filename: string): Promise<PackageManifest> { let source: string; try { source = await readFile(filename, 'utf8'); } catch (error) { throw new Error(`read package manifest ${JSON.stringify(filename)}: ${errorMessage(error)}`, { cause: error }); } let manifest: unknown; try { manifest = JSON.parse(source); } catch (error) { throw new Error(`parse package manifest ${JSON.stringify(filename)}: ${errorMessage(error)}`, { cause: error }); } if (manifest === null || typeof manifest !== 'object' || Array.isArray(manifest)) throw new Error(`package manifest ${JSON.stringify(filename)} must contain a JSON object`); return manifest as PackageManifest; }
function packageCoordinates(manifest: PackageManifest, filename: string): { name: string; version: string } { validatePackageName(manifest.name, `package manifest ${JSON.stringify(filename)}`); if (typeof manifest.version !== 'string' || !PACKAGE_VERSION.test(manifest.version)) throw new Error(`package manifest ${JSON.stringify(filename)} has invalid semantic version ${JSON.stringify(manifest.version)}`); return { name: manifest.name, version: manifest.version }; }
function dependencyNames(manifest: PackageManifest, filename: string): string[] { if (manifest.dependencies === undefined) return []; if (manifest.dependencies === null || typeof manifest.dependencies !== 'object' || Array.isArray(manifest.dependencies)) throw new Error(`package manifest ${JSON.stringify(filename)} dependencies must be an object`); const dependencies = manifest.dependencies as Record<string, unknown>; const names = Object.keys(dependencies).sort(); for (const name of names) { validatePackageName(name, `dependencies in ${JSON.stringify(filename)}`); if (typeof dependencies[name] !== 'string' || dependencies[name] === '') throw new Error(`dependency ${JSON.stringify(name)} in ${JSON.stringify(filename)} must have a non-empty string specifier`); } return names; }
function validatePackageName(name: unknown, context: string): asserts name is string { if (typeof name !== 'string' || !PACKAGE_NAME.test(name)) throw new Error(`${context} has invalid npm package name ${JSON.stringify(name)}`); }
function hasLocusMetadata(manifest: PackageManifest): boolean { return Object.prototype.hasOwnProperty.call(manifest, 'locus'); }
async function validateLocusEntry(manifest: PackageManifest, root: string, packageJson: string): Promise<string> { const locus = manifest.locus; if (locus === null || typeof locus !== 'object' || Array.isArray(locus) || typeof (locus as { entry?: unknown }).entry !== 'string' || (locus as { entry: string }).entry === '') throw new Error(`Locus package ${JSON.stringify(packageJson)} must declare a non-empty locus.entry string`); const declared = (locus as { entry: string }).entry; if (declared.includes('\\') || path.posix.isAbsolute(declared) || /^[A-Za-z]:\//.test(declared)) throw new Error(`locus.entry ${JSON.stringify(declared)} in ${JSON.stringify(packageJson)} must be a relative slash path`); const entry = path.posix.normalize(declared.replace(/^\.\//, '')); if (entry === '.' || entry === '..' || entry.startsWith('../')) throw new Error(`locus.entry ${JSON.stringify(declared)} in ${JSON.stringify(packageJson)} escapes the package root`); if (!SCOPE_MANIFEST_NAMES.has(path.posix.basename(entry))) throw new Error(`locus.entry ${JSON.stringify(declared)} in ${JSON.stringify(packageJson)} is not a Scope manifest`); const entryPath = path.join(root, ...entry.split('/')); let entryInfo; let canonicalEntry; try { [entryInfo, canonicalEntry] = await Promise.all([stat(entryPath), realpath(entryPath)]); } catch (error) { throw new Error(`inspect locus.entry ${JSON.stringify(entryPath)}: ${errorMessage(error)}`, { cause: error }); } if (!entryInfo.isFile() || escapesRoot(root, canonicalEntry)) throw new Error(`locus.entry ${JSON.stringify(declared)} in ${JSON.stringify(packageJson)} must be a file inside the package root`); return entry; }
function escapesRoot(root: string, candidate: string): boolean { const relative = path.relative(root, candidate); return relative === '..' || relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative); }
function errorMessage(error: unknown): string { return error instanceof Error ? error.message : String(error); }
function errorCode(error: unknown): unknown { return typeof error === 'object' && error !== null && 'cause' in error && typeof error.cause === 'object' && error.cause !== null && 'code' in error.cause ? error.cause.code : typeof error === 'object' && error !== null && 'code' in error ? error.code : undefined; }
