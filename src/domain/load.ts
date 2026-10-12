import { realpath, stat } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { MANIFEST_FILE_NAMES } from './discover.js';
import { decodeScope } from './decode.js';
import { validateWorkspace } from './validate.js';
import { Workspace, type Resolver, type ScopeKey, type Source } from './types.js';

export class LocalResolver implements Resolver {
  async resolve(from: Source, reference: string): Promise<Source> {
    let parsed: URL;
    try { parsed = new URL(from.key); } catch { throw new Error(`resolve local import ${JSON.stringify(reference)} from unsupported source ${JSON.stringify(from.key)}`); }
    if (parsed.protocol !== 'file:' || !from.key.startsWith('file://')) throw new Error(`resolve local import ${JSON.stringify(reference)} from unsupported source ${JSON.stringify(from.key)}`);
    if (reference.includes('://')) throw new Error(`resolve local import ${JSON.stringify(reference)} from ${JSON.stringify(from.key)}: unsupported source scheme ${JSON.stringify(reference.slice(0, reference.indexOf('://')))}`);
    const target = path.isAbsolute(reference) ? reference : path.join(from.localPath, reference);
    return newLocalSource(target);
  }
}

export async function findScope(start: string): Promise<string> {
  let current = path.resolve(start);
  const info = await stat(current).catch((error: unknown) => { throw new Error(`inspect start path ${JSON.stringify(current)}: ${(error as Error).message}`, { cause: error }); });
  if (!info.isDirectory()) current = path.dirname(current);
  while (true) {
    for (const name of MANIFEST_FILE_NAMES) {
      const candidate = path.join(current, name);
      try { const candidateInfo = await stat(candidate); if (!candidateInfo.isDirectory()) return current; }
      catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw new Error(`inspect ${candidate}: ${(error as Error).message}`, { cause: error }); }
    }
    const parent = path.dirname(current);
    if (parent === current) break;
    current = parent;
  }
  throw new Error(`no locus manifest found from ${JSON.stringify(start)} to the filesystem root`);
}

export async function canonicalScopeDirectory(input: string): Promise<string> {
  const absolute = path.resolve(input);
  let info;
  try { info = await stat(absolute); } catch (error) { throw new Error(`scope source ${JSON.stringify(absolute)}: ${(error as Error).message}`, { cause: error }); }
  if (!info.isDirectory()) throw new Error(`scope source ${JSON.stringify(absolute)} is not a directory`);
  try { return await realpath(absolute); } catch (error) { throw new Error(`resolve scope source ${JSON.stringify(absolute)}: ${(error as Error).message}`, { cause: error }); }
}

export async function newLocalSource(input: string): Promise<Source> {
  const localPath = await canonicalScopeDirectory(input);
  return { key: pathToFileURL(localPath).toString(), localPath };
}

export async function readSource(source: Source) {
  const normalized = await normalizeSource(source);
  return decodeScope(normalized);
}
interface PendingScope { readonly source: Source; readonly importer?: ScopeKey; readonly alias?: string; readonly sourceValue?: string; }

export async function checkSource(source: Source): Promise<void> { await readSource(source); }

export async function load(rootInput: Source, resolver: Resolver): Promise<Workspace> {
  if (!resolver) throw new Error('load root scope: resolver is required');
  let root: Source;
  try { root = await normalizeSource(rootInput); }
  catch (error) { throw new Error(`load root scope: ${(error as Error).message}`, { cause: error }); }
  const workspace = new Workspace(root.key);
  const sourcePaths = new Map<ScopeKey, string>([[root.key, root.localPath]]);
  const queue: PendingScope[] = [{ source: root }];
  while (queue.length > 0) {
    const pending = queue.shift() as PendingScope;
    let source: Source;
    try { source = await normalizeSource(pending.source); }
    catch (error) { throw wrapImportError(workspace, pending, error); }
    const previous = sourcePaths.get(source.key);
    if (previous !== undefined && previous !== source.localPath) throw wrapImportError(workspace, pending, new Error(`scope key ${JSON.stringify(source.key)} resolves to both ${JSON.stringify(previous)} and ${JSON.stringify(source.localPath)}`));
    sourcePaths.set(source.key, source.localPath);
    if (workspace.scopes.has(source.key)) continue;
    let loaded;
    try { loaded = await decodeScope(source); }
    catch (error) { throw wrapImportError(workspace, pending, error); }
    workspace.scopes.set(source.key, loaded);
    const aliases = Object.keys(loaded.manifest.imports).sort();
    for (const alias of aliases) {
      const sourceValue = loaded.manifest.imports[alias] as string;
      let target: Source;
      try { target = await resolver.resolve(source, sourceValue); }
      catch (error) { throw new Error(`${path.join(loaded.localPath, loaded.manifestFile)}: import ${JSON.stringify(alias)} (${JSON.stringify(sourceValue)}): ${(error as Error).message}`, { cause: error }); }
      try { target = await normalizeSource(target); }
      catch (error) { throw new Error(`${path.join(loaded.localPath, loaded.manifestFile)}: import ${JSON.stringify(alias)} (${JSON.stringify(sourceValue)}): ${(error as Error).message}`, { cause: error }); }
      const existing = sourcePaths.get(target.key);
      if (existing !== undefined && existing !== target.localPath) throw new Error(`${path.join(loaded.localPath, loaded.manifestFile)}: import ${JSON.stringify(alias)} (${JSON.stringify(sourceValue)}): scope key ${JSON.stringify(target.key)} resolves to both ${JSON.stringify(existing)} and ${JSON.stringify(target.localPath)}`);
      sourcePaths.set(target.key, target.localPath);
      loaded.imports.set(alias, target.key);
      if (!workspace.scopes.has(target.key)) queue.push({ source: target, importer: loaded.key, alias, sourceValue });
    }
  }
  validateWorkspace(workspace);
  return workspace;
}

function wrapImportError(workspace: Workspace, pending: PendingScope, error: unknown): Error {
  if (!pending.importer) return error instanceof Error ? error : new Error(String(error));
  const importer = workspace.scopes.get(pending.importer);
  const message = error instanceof Error ? error.message : String(error);
  return new Error(`${importer ? path.join(importer.localPath, importer.manifestFile) : pending.importer}: import ${JSON.stringify(pending.alias)} (${JSON.stringify(pending.sourceValue)}): ${message}`, { cause: error });
}

export async function normalizeSource(source: Source): Promise<Source> {
  if (!source.key) throw new Error('scope source key is required');
  if (!source.localPath?.trim()) throw new Error(`scope source ${JSON.stringify(source.key)} local path is required`);
  return { key: source.key, localPath: await canonicalScopeDirectory(source.localPath) };
}

export const FindScope = findScope;
export const NewLocalSource = newLocalSource;
export const ReadSource = readSource;
export const CheckSource = checkSource;
export const Load = load;
export const CanonicalScopeDirectory = canonicalScopeDirectory;
