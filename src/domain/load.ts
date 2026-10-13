import { realpath, stat } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { MANIFEST_FILE_NAMES } from './discover.js';
import { createError, ERROR_REASON_CODES, sourceError } from './errors.js';
import { decodeScope } from './decode.js';
import { validateWorkspace } from './validate.js';
import { Workspace, type Resolver, type ScopeKey, type Source } from './types.js';
import type { ErrorReason } from './errors.js';

export class LocalResolver implements Resolver {
  async resolve(from: Source, reference: string): Promise<Source> {
    let parsed: URL;
    try { parsed = new URL(from.key); }
    catch (error) { throw createError('scope.import_unresolved', 'Import source cannot be resolved from the current source', { reference: safeReference(reference) }, error); }
    if (parsed.protocol !== 'file:' || !from.key.startsWith('file://')) throw createError('scope.import_unresolved', 'Import source is not a local file source', { reference: safeReference(reference) });
    if (reference.includes('://')) throw createError('scope.import_unresolved', 'Import source scheme is not supported', { reference: safeReference(reference) });
    const target = path.isAbsolute(reference) ? reference : path.join(from.localPath, reference);
    return newLocalSource(target);
  }
}

export async function findScope(start: string): Promise<string> {
  let current = path.resolve(start);
  let info;
  try { info = await stat(current); }
  catch (error) { throw sourceError(error, 'Cannot inspect the starting path'); }
  if (!info.isDirectory()) current = path.dirname(current);
  while (true) {
    for (const name of MANIFEST_FILE_NAMES) {
      const candidate = path.join(current, name);
      try { const candidateInfo = await stat(candidate); if (!candidateInfo.isDirectory()) return current; }
      catch (error) {
        if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw sourceError(error, 'Cannot inspect scope source', { file: name });
      }
    }
    const parent = path.dirname(current);
    if (parent === current) break;
    current = parent;
  }
  throw createError('scope.source_not_found', 'No Locus manifest was found from the starting path to the filesystem root');
}

export async function canonicalScopeDirectory(input: string): Promise<string> {
  const absolute = path.resolve(input);
  let info;
  try { info = await stat(absolute); }
  catch (error) { throw sourceError(error, 'Cannot inspect scope source'); }
  if (!info.isDirectory()) throw createError('scope.source_invalid', 'Scope source is not a directory');
  try { return await realpath(absolute); }
  catch (error) { throw sourceError(error, 'Cannot resolve scope source'); }
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
  if (!resolver) throw createError('scope.internal', 'Scope loader resolver is required');
  let root: Source;
  try { root = await normalizeSource(rootInput); }
  catch (error) { throw wrapImportError({ source: rootInput }, error); }
  const workspace = new Workspace(root.key);
  const sourcePaths = new Map<ScopeKey, string>([[root.key, root.localPath]]);
  const queue: PendingScope[] = [{ source: root }];
  while (queue.length > 0) {
    const pending = queue.shift() as PendingScope;
    let source: Source;
    try { source = await normalizeSource(pending.source); }
    catch (error) { throw wrapImportError(pending, error); }
    const previous = sourcePaths.get(source.key);
    if (previous !== undefined && previous !== source.localPath) {
      throw wrapImportError(pending, createError('scope.source_identity_conflict', 'A source identity resolves to conflicting locations'));
    }
    sourcePaths.set(source.key, source.localPath);
    if (workspace.scopes.has(source.key)) continue;
    let loaded;
    try { loaded = await decodeScope(source); }
    catch (error) { throw wrapImportError(pending, error); }
    workspace.scopes.set(source.key, loaded);
    const aliases = Object.keys(loaded.manifest.imports).sort();
    for (const alias of aliases) {
      const sourceValue = loaded.manifest.imports[alias] as string;
      let target: Source;
      try { target = await resolver.resolve(source, sourceValue); }
      catch (error) { throw wrapImportError({ source, importer: loaded.key, alias, sourceValue }, error); }
      try { target = await normalizeSource(target); }
      catch (error) { throw wrapImportError({ source: target, importer: loaded.key, alias, sourceValue }, error); }
      const existing = sourcePaths.get(target.key);
      if (existing !== undefined && existing !== target.localPath) {
        throw createError('scope.source_identity_conflict', 'An import source identity resolves to conflicting locations', { reference: safeReference(sourceValue) });
      }
      sourcePaths.set(target.key, target.localPath);
      loaded.imports.set(alias, target.key);
      if (!workspace.scopes.has(target.key)) queue.push({ source: target, importer: loaded.key, alias, sourceValue });
    }
  }
  validateWorkspace(workspace);
  return workspace;
}

type ClassifiedError = { readonly reason: ErrorReason };
function wrapImportError(pending: PendingScope, error: unknown): ClassifiedError {
  if (error && typeof error === 'object' && 'reason' in error && typeof error.reason === 'string' && Object.hasOwn(ERROR_REASON_CODES, error.reason)) {
    const reason = error.reason as ErrorReason;
    if (!pending.importer) return error as ClassifiedError;
    if (reason === 'scope.internal') return error as ClassifiedError;
    const details = pending.sourceValue === undefined ? undefined : { reference: safeReference(pending.sourceValue) };
    return createError(reason, `Import ${JSON.stringify(pending.alias)} could not be loaded`, details, error);
  }
  if (!pending.importer) return sourceError(error, 'Scope source could not be loaded');
  const details = pending.sourceValue === undefined ? undefined : { reference: safeReference(pending.sourceValue) };
  return createError('scope.internal', `Import ${JSON.stringify(pending.alias)} failed unexpectedly`, details, error);
}

function safeReference(reference: string): string {
  return path.isAbsolute(reference) ? path.basename(reference) : reference;
}

export async function normalizeSource(source: Source): Promise<Source> {
  if (!source?.key) throw createError('scope.source_invalid', 'Scope source key is required');
  if (!source.localPath?.trim()) throw createError('scope.source_invalid', 'Scope source local path is required');
  return { key: source.key, localPath: await canonicalScopeDirectory(source.localPath) };
}

export const FindScope = findScope;
export const NewLocalSource = newLocalSource;
export const ReadSource = readSource;
export const CheckSource = checkSource;
export const Load = load;
export const CanonicalScopeDirectory = canonicalScopeDirectory;
