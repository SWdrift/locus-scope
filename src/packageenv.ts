import { realpath, stat } from 'node:fs/promises';
import path from 'node:path';
import { checkSource, load as loadWorkspace, LocalResolver, newLocalSource, type Resolver, type ScopeKey, type Source, type Workspace } from './domain/index.js';

export type Identity = string;
export interface PackageDescriptor { root: string; entry: string; dependencies: Record<string, Identity>; }
export interface Environment { rootScope: string; rootDependencies: Record<string, Identity>; packages: Record<Identity, PackageDescriptor>; }

interface PreparedResolver extends Resolver { readonly rootDependencies: Record<string, Identity>; readonly packages: Record<Identity, PackageDescriptor>; }

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
    if (!identity.trim()) throw new Error('package identity must not be empty');
    const packageInfo = Object.prototype.hasOwnProperty.call(environment.packages, identity) ? environment.packages[identity] : undefined;
    if (!packageInfo) throw new Error(`package ${JSON.stringify(identity)} descriptor is required`);
    const secure = await secureEntry(packageInfo.root, packageInfo.entry);
    const dependencies = { ...packageInfo.dependencies };
    const source: Source = { key: identity as ScopeKey, localPath: path.dirname(path.join(secure.root, ...secure.entry.split('/'))) };
    try { await checkSource(source); }
    catch (error) { throw new Error(`package ${JSON.stringify(identity)} Scope source: ${(error as Error).message}`, { cause: error }); }
    packages[identity] = { root: secure.root, entry: secure.entry, dependencies };
    packageSources.set(identity, source);
  }
  const rootDependencies = { ...environment.rootDependencies };
  validateEdges('root', rootDependencies, packages);
  for (const identity of identities) validateEdges(`package ${JSON.stringify(identity)}`, packages[identity]?.dependencies ?? {}, packages);
  const localResolver = new LocalResolver();
  return {
    rootDependencies,
    packages,
    async resolve(from: Source, reference: string): Promise<Source> {
      const pkg = packages[from.key];
      if (isLocalImport(reference)) {
        if (pkg) throw new Error(`distributed package ${JSON.stringify(from.key)} cannot import local Scope ${JSON.stringify(reference)}`);
        return await localResolver.resolve(from, reference);
      }
      const dependencies = pkg?.dependencies ?? rootDependencies;
      const importer = pkg ? `package ${JSON.stringify(from.key)}` : 'local root';
      const identity = dependencies[reference];
      if (!identity) throw new Error(`${importer} does not declare package import ${JSON.stringify(reference)} in the resolved environment`);
      const target = packageSources.get(identity);
      if (!target) throw new Error(`${importer} package import ${JSON.stringify(reference)} targets unavailable package ${JSON.stringify(identity)}`);
      return target;
    },
  };
}
function validateEdges(importer: string, dependencies: Record<string, Identity>, packages: Record<Identity, PackageDescriptor>): void {
  for (const name of Object.keys(dependencies).sort()) {
    if (!name) throw new Error(`${importer} dependency name must not be empty`);
    const identity = dependencies[name];
    if (!identity || !Object.prototype.hasOwnProperty.call(packages, identity)) throw new Error(`${importer} dependency ${JSON.stringify(name)} targets missing package ${JSON.stringify(identity)}`);
  }
}

export async function secureEntry(root: string, entry: string): Promise<{ root: string; entry: string }> {
  if (!root || !entry || path.isAbsolute(entry) || entry.includes('\\')) throw new Error(`entry ${JSON.stringify(entry)} must be a relative slash path`);
  let canonicalRoot: string;
  try { canonicalRoot = await realpath(path.resolve(root)); }
  catch (error) { throw new Error(`resolve package root: ${(error as Error).message}`, { cause: error }); }
  const cleanEntry = path.posix.normalize(entry.replace(/^\.\//u, ''));
  if (cleanEntry === '.' || cleanEntry === '..' || cleanEntry.startsWith('../')) throw new Error(`entry ${JSON.stringify(entry)} must be normalized and remain inside the package root`);
  if (!['locus.yaml', 'locus.yml', 'locus.json'].includes(path.posix.basename(cleanEntry))) throw new Error(`entry ${JSON.stringify(entry)} must name a Scope manifest`);
  let candidate: string;
  try { candidate = await realpath(path.join(canonicalRoot, ...cleanEntry.split('/'))); }
  catch (error) { throw new Error(`resolve entry ${JSON.stringify(entry)}: ${(error as Error).message}`, { cause: error }); }
  const relative = path.relative(canonicalRoot, candidate);
  if (!relative || relative === '..' || relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative)) throw new Error(`entry ${JSON.stringify(entry)} escapes package root`);
  try { const info = await stat(candidate); if (!info.isFile()) throw new Error(`entry ${JSON.stringify(entry)} is not a readable file`); }
  catch (error) { if ((error as Error).message.startsWith('entry ')) throw error; throw new Error(`inspect entry ${JSON.stringify(entry)}: ${(error as Error).message}`, { cause: error }); }
  return { root: path.normalize(canonicalRoot), entry: cleanEntry };
}

function isLocalImport(reference: string): boolean { return reference.startsWith('./') || reference.startsWith('../') || path.isAbsolute(reference); }
export const Load = load;
export const SecureEntry = secureEntry;
