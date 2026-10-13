import { readFile, realpath, stat } from "node:fs/promises";
import { createRequire } from "node:module";
import path from "node:path";
import { pathToFileURL } from "node:url";
import { createError, sourceError } from "./domain/errors.js";

const PACKAGE_NAME = /^(?:@[a-z0-9][a-z0-9._~-]*\/)?[a-z0-9][a-z0-9._~-]*$/;
const PACKAGE_VERSION =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;
const SCOPE_MANIFEST_NAMES = new Set(["locus.yaml", "locus.yml", "locus.json"]);

type PackageManifest = {
  name?: unknown;
  version?: unknown;
  dependencies?: unknown;
  locus?: unknown;
  [key: string]: unknown;
};
type PackageRecord = { packageJson: string; manifest: PackageManifest };
type Occurrence = { identity: string; root: string; entry: string; dependencies: Record<string, string> | null };
export type PackageDescriptor = { root: string; entry: string; dependencies: Record<string, string> };
export type PackageEnvironment = {
  rootScope: string;
  rootDependencies: Record<string, string>;
  packages: Record<string, PackageDescriptor>;
};

export async function buildPackageEnvironment(scopeRoot: string): Promise<PackageEnvironment> {
  const canonicalScopeRoot = await canonicalPath(scopeRoot);
  const rootPackageJson = path.join(canonicalScopeRoot, "package.json");
  const rootManifest = await readOptionalPackageJson(rootPackageJson);
  if (rootManifest === null) return { rootScope: canonicalScopeRoot, rootDependencies: {}, packages: {} };
  const rootDependencies = dependencyNames(rootManifest, rootPackageJson);
  const occurrences = new Map<string, Occurrence>();
  const visitLocusPackage = async (record: PackageRecord): Promise<string> => {
    const root = await canonicalPath(path.dirname(record.packageJson));
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
    if (record !== null && hasLocusMetadata(record.manifest))
      directDependencies[dependency] = await visitLocusPackage(record);
  }
  const byIdentity = new Map<string, Occurrence>();
  for (const occurrence of occurrences.values()) {
    if (occurrence.dependencies === null)
      throw createError("scope.internal", "dependency traversal did not complete", { reference: occurrence.identity });
    const existing = byIdentity.get(occurrence.identity);
    if (!existing) byIdentity.set(occurrence.identity, occurrence);
    else if (
      existing.entry !== occurrence.entry ||
      JSON.stringify(existing.dependencies) !== JSON.stringify(occurrence.dependencies)
    )
      throw createError(
        "scope.package_invalid",
        "conflicting npm package copies have different entries or resolved dependencies",
        { reference: occurrence.identity }
      );
    else if (occurrence.root < existing.root) byIdentity.set(occurrence.identity, occurrence);
  }
  const packages: Record<string, PackageDescriptor> = {};
  for (const identity of [...byIdentity.keys()].sort()) {
    const occurrence = byIdentity.get(identity)!;
    packages[identity] = { root: occurrence.root, entry: occurrence.entry, dependencies: occurrence.dependencies! };
  }
  return { rootScope: canonicalScopeRoot, rootDependencies: directDependencies, packages };
}

async function resolveDependency(importerPackageJson: string, dependency: string): Promise<PackageRecord | null> {
  validatePackageName(dependency, "dependency", "dependencies");
  const importerRequire = createRequire(pathToFileURL(importerPackageJson));
  let packageJson: string;
  let packageJsonResolutionError: unknown;
  try {
    packageJson = importerRequire.resolve(`${dependency}/package.json`);
  } catch (error) {
    packageJsonResolutionError = error;
  }
  if (packageJsonResolutionError === undefined) return readResolvedPackage(packageJson!);
  let entry: string;
  try {
    entry = importerRequire.resolve(dependency);
  } catch (error) {
    if (errorCode(error) === "MODULE_NOT_FOUND" || errorCode(error) === "ERR_PACKAGE_PATH_NOT_EXPORTED") return null;
    throw sourceError(error, "resolve dependency failed", { reference: dependency });
  }
  const record = await findPackageFromEntry(entry, dependency, importerPackageJson);
  if (hasLocusMetadata(record.manifest))
    throw createError(
      "scope.package_invalid",
      'Locus package must export "./package.json"',
      { reference: dependency },
      packageJsonResolutionError
    );
  return record;
}

async function findPackageFromEntry(
  entry: string,
  dependency: string,
  _importerPackageJson: string
): Promise<PackageRecord> {
  let current = path.dirname(await canonicalPath(entry));
  while (true) {
    const packageJson = path.join(current, "package.json");
    const manifest = await readOptionalPackageJson(packageJson);
    if (manifest !== null) {
      validatePackageName(manifest.name, "package manifest", "name");
      if (manifest.name === dependency) return { packageJson: await canonicalPath(packageJson), manifest };
    }
    const parent = path.dirname(current);
    if (parent === current)
      throw createError("scope.dependency_unresolved", "resolved dependency has no package manifest", {
        reference: dependency
      });
    current = parent;
  }
}
async function canonicalPath(filename: string): Promise<string> {
  try {
    return await realpath(filename);
  } catch (error) {
    throw sourceError(error, "resolve package source failed");
  }
}
async function readResolvedPackage(packageJson: string): Promise<PackageRecord> {
  const canonical = await canonicalPath(packageJson);
  const manifest = await readPackageJson(canonical);
  validatePackageName(manifest.name, "package manifest", "name");
  return { packageJson: canonical, manifest };
}
async function readOptionalPackageJson(filename: string): Promise<PackageManifest | null> {
  try {
    return await readPackageJson(filename);
  } catch (error) {
    if (errorCode(error) === "ENOENT") return null;
    throw error;
  }
}
async function readPackageJson(filename: string): Promise<PackageManifest> {
  let source: string;
  try {
    source = await readFile(filename, "utf8");
  } catch (error) {
    throw sourceError(error, "read package manifest failed", { file: "package.json" });
  }
  let manifest: unknown;
  try {
    manifest = JSON.parse(source);
  } catch (error) {
    throw createError("scope.package_invalid", "parse package manifest failed", { file: "package.json" }, error);
  }
  if (manifest === null || typeof manifest !== "object" || Array.isArray(manifest))
    throw createError("scope.package_invalid", "package manifest must contain a JSON object", { file: "package.json" });
  return manifest as PackageManifest;
}
function packageCoordinates(manifest: PackageManifest, _filename: string): { name: string; version: string } {
  validatePackageName(manifest.name, "package manifest", "name");
  if (typeof manifest.version !== "string" || !PACKAGE_VERSION.test(manifest.version))
    throw createError("scope.package_invalid", "package manifest has an invalid semantic version", {
      field: "version"
    });
  return { name: manifest.name, version: manifest.version };
}
function dependencyNames(manifest: PackageManifest, _filename: string): string[] {
  if (manifest.dependencies === undefined) return [];
  if (
    manifest.dependencies === null ||
    typeof manifest.dependencies !== "object" ||
    Array.isArray(manifest.dependencies)
  )
    throw createError("scope.package_invalid", "package dependencies must be an object", { field: "dependencies" });
  const dependencies = manifest.dependencies as Record<string, unknown>;
  const names = Object.keys(dependencies).sort();
  for (const name of names) {
    validatePackageName(name, "dependency", "dependencies");
    if (typeof dependencies[name] !== "string" || dependencies[name] === "")
      throw createError("scope.package_invalid", "dependency must have a non-empty string specifier", {
        field: "dependencies",
        reference: name
      });
  }
  return names;
}
function validatePackageName(name: unknown, context: string, field: string): asserts name is string {
  if (typeof name !== "string" || !PACKAGE_NAME.test(name))
    throw createError("scope.package_invalid", `${context} has an invalid npm package name`, { field });
}
function hasLocusMetadata(manifest: PackageManifest): boolean {
  return Object.prototype.hasOwnProperty.call(manifest, "locus");
}
async function validateLocusEntry(manifest: PackageManifest, root: string, _packageJson: string): Promise<string> {
  const locus = manifest.locus;
  if (
    locus === null ||
    typeof locus !== "object" ||
    Array.isArray(locus) ||
    typeof (locus as { entry?: unknown }).entry !== "string" ||
    (locus as { entry: string }).entry === ""
  )
    throw createError("scope.package_invalid", "Locus package must declare a non-empty locus.entry string", {
      field: "locus.entry"
    });
  const declared = (locus as { entry: string }).entry;
  if (declared.includes("\\") || path.posix.isAbsolute(declared) || /^[A-Za-z]:\//.test(declared))
    throw createError("scope.package_invalid", "locus.entry must be a relative slash path", { field: "locus.entry" });
  const entry = path.posix.normalize(declared.replace(/^\.\//, ""));
  if (entry === "." || entry === ".." || entry.startsWith("../"))
    throw createError("scope.package_invalid", "locus.entry escapes the package root", { field: "locus.entry" });
  if (!SCOPE_MANIFEST_NAMES.has(path.posix.basename(entry)))
    throw createError("scope.package_invalid", "locus.entry must name a Scope manifest", { field: "locus.entry" });
  const entryPath = path.join(root, ...entry.split("/"));
  let entryInfo;
  let canonicalEntry;
  try {
    [entryInfo, canonicalEntry] = await Promise.all([stat(entryPath), realpath(entryPath)]);
  } catch (error) {
    throw sourceError(error, "inspect locus.entry failed", { file: entry });
  }
  if (!entryInfo.isFile() || escapesRoot(root, canonicalEntry))
    throw createError("scope.package_invalid", "locus.entry must be a file inside the package root", {
      field: "locus.entry"
    });
  return entry;
}
function escapesRoot(root: string, candidate: string): boolean {
  const relative = path.relative(root, candidate);
  return relative === ".." || relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative);
}
function errorCode(error: unknown): unknown {
  return typeof error === "object" &&
    error !== null &&
    "cause" in error &&
    typeof error.cause === "object" &&
    error.cause !== null &&
    "code" in error.cause
    ? error.cause.code
    : typeof error === "object" && error !== null && "code" in error
      ? error.code
      : undefined;
}
