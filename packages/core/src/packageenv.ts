import { realpath, stat } from "node:fs/promises";
import path from "node:path";
import {
  checkSource,
  load as loadWorkspace,
  LocalResolver,
  newLocalSource,
  type Resolver,
  type ScopeKey,
  type Source,
  type Workspace
} from "./domain/index.js";
import { createError, sourceError } from "./domain/errors.js";

export type Identity = string;
export interface PackageDescriptor {
  root: string;
  entry: string;
  dependencies: Record<string, Identity>;
}
export interface Environment {
  rootScope: string;
  rootDependencies: Record<string, Identity>;
  packages: Record<Identity, PackageDescriptor>;
}

interface PreparedResolver extends Resolver {
  readonly rootDependencies: Record<string, Identity>;
  readonly packages: Record<Identity, PackageDescriptor>;
}

export async function load(environment: Environment): Promise<Workspace> {
  const root = await newLocalSource(environment.rootScope);
  const resolver = await newResolver(environment);
  return loadWorkspace(root, resolver);
}

async function newResolver(environment: Environment): Promise<PreparedResolver> {
  const packages = Object.create(null) as Record<Identity, PackageDescriptor>;
  const packageSources = new Map<Identity, Source>();
  const identities = Object.keys(environment.packages).sort();
  for (const identity of identities) {
    if (!identity.trim()) throw createError("scope.package_invalid", "package identity must not be empty");
    const packageInfo = Object.prototype.hasOwnProperty.call(environment.packages, identity)
      ? environment.packages[identity]
      : undefined;
    if (!packageInfo)
      throw createError("scope.package_invalid", "package descriptor is required", { reference: identity });
    const secure = await secureEntry(packageInfo.root, packageInfo.entry);
    const dependencies = { ...packageInfo.dependencies };
    const source: Source = {
      key: identity as ScopeKey,
      localPath: path.dirname(path.join(secure.root, ...secure.entry.split("/")))
    };
    try {
      await checkSource(source);
    } catch (error) {
      throw sourceError(error, "package Scope source is unavailable", { reference: identity });
    }
    packages[identity] = { root: secure.root, entry: secure.entry, dependencies };
    packageSources.set(identity, source);
  }
  const rootDependencies = { ...environment.rootDependencies };
  validateEdges("root", rootDependencies, packages);
  for (const identity of identities)
    validateEdges(`package ${JSON.stringify(identity)}`, packages[identity]?.dependencies ?? {}, packages);
  const localResolver = new LocalResolver();
  return {
    rootDependencies,
    packages,
    async resolve(from: Source, reference: string): Promise<Source> {
      const pkg = packages[from.key];
      if (isLocalImport(reference)) {
        if (pkg) throw createError("scope.package_invalid", "distributed packages cannot import local Scopes");
        return await localResolver.resolve(from, reference);
      }
      const dependencies = pkg?.dependencies ?? rootDependencies;
      const importer = pkg ? `package ${JSON.stringify(from.key)}` : "local root";
      const identity = dependencies[reference];
      if (!identity)
        throw createError("scope.dependency_unresolved", `${importer} does not declare the package import`, {
          reference
        });
      const target = packageSources.get(identity);
      if (!target)
        throw createError("scope.dependency_unresolved", `${importer} package import targets an unavailable package`, {
          reference
        });
      return target;
    }
  };
}
function validateEdges(
  importer: string,
  dependencies: Record<string, Identity>,
  packages: Record<Identity, PackageDescriptor>
): void {
  for (const name of Object.keys(dependencies).sort()) {
    if (!name) throw createError("scope.package_invalid", `${importer} dependency name must not be empty`);
    const identity = dependencies[name];
    if (!identity || !Object.prototype.hasOwnProperty.call(packages, identity))
      throw createError("scope.dependency_unresolved", `${importer} dependency targets a missing package`, {
        reference: name
      });
  }
}

export async function secureEntry(root: string, entry: string): Promise<{ root: string; entry: string }> {
  if (!root || !entry || path.isAbsolute(entry) || entry.includes("\\"))
    throw createError("scope.package_invalid", "entry must be a relative slash path", { field: "entry" });
  let canonicalRoot: string;
  try {
    canonicalRoot = await realpath(path.resolve(root));
  } catch (error) {
    throw sourceError(error, "resolve package root failed");
  }
  const cleanEntry = path.posix.normalize(entry.replace(/^\.\//u, ""));
  if (cleanEntry === "." || cleanEntry === ".." || cleanEntry.startsWith("../"))
    throw createError("scope.package_invalid", "entry must remain inside the package root", { field: "entry" });
  if (!["locus.yaml", "locus.yml", "locus.json"].includes(path.posix.basename(cleanEntry)))
    throw createError("scope.package_invalid", "entry must name a Scope manifest", { field: "entry" });
  let candidate: string;
  try {
    candidate = await realpath(path.join(canonicalRoot, ...cleanEntry.split("/")));
  } catch (error) {
    throw sourceError(error, "resolve package entry failed", { file: cleanEntry });
  }
  const relative = path.relative(canonicalRoot, candidate);
  if (!relative || relative === ".." || relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative))
    throw createError("scope.package_invalid", "entry escapes package root", { field: "entry" });
  let info;
  try {
    info = await stat(candidate);
  } catch (error) {
    throw sourceError(error, "inspect package entry failed", { file: cleanEntry });
  }
  if (!info.isFile()) throw createError("scope.package_invalid", "entry is not a file", { file: cleanEntry });
  return { root: path.normalize(canonicalRoot), entry: cleanEntry };
}

function isLocalImport(reference: string): boolean {
  return reference.startsWith("./") || reference.startsWith("../") || path.isAbsolute(reference);
}
export const Load = load;
export const SecureEntry = secureEntry;
