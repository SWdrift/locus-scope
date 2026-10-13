import { mkdir, readFile, lstat, realpath, rm } from 'node:fs/promises';
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import YAML from 'yaml';
import writeFileAtomic from 'write-file-atomic';
import type { EntityKey, Workspace } from '../domain/index.js';
import { ApplicationError, createError, sourceError } from '../domain/errors.js';
import type { MutationRequest, MutationResult, WorkspaceReloader } from './types.js';
import { ApplicationService, cloneObject, cloneValue } from './service.js';

interface MutableDocument { group?: string; entities: Record<string, unknown>[]; relations: Record<string, unknown>[] }

function mutationInvalid(message: string, cause?: unknown): ApplicationError {
  return createError('scope.mutation_input_invalid', message, undefined, cause);
}
function mutationNotFound(message: string, cause?: unknown): ApplicationError {
  return createError('scope.object_not_found', message, undefined, cause);
}
function mutationInvalidData(message: string, cause?: unknown): ApplicationError {
  return createError('scope.mutation_invalid', message, undefined, cause);
}

function sameKey(left: EntityKey, right: EntityKey): boolean {
  return left.scope === right.scope && left.id === right.id;
}
ApplicationService.prototype.mutate = async function mutate(this: ApplicationService, request: MutationRequest, reload: WorkspaceReloader): Promise<{ result: MutationResult; workspace: Workspace }> {
  const root = this.workspace.scopes.get(this.workspace.root);
  if (!root) throw createError('scope.internal', 'root scope not found');
  if (request.kind !== 'entity' && request.kind !== 'relation') throw mutationInvalid(`unknown mutation kind ${JSON.stringify(request.kind)}`);
  if (request.action !== 'add' && request.action !== 'remove' && request.action !== 'set' && request.action !== 'unset') throw mutationInvalid(`unknown mutation action ${JSON.stringify(request.action)}`);
  const target = await mutationTarget(this, root.localPath, request);
  const loaded = await readMutableDocument(target.path);
  let object: Record<string, unknown>;
  if (request.kind === 'entity') object = mutateEntity(loaded.document, request, target.localId);
  else object = mutateRelation(loaded.document, request, target.relationIndex);
  const encoded = encodeMutableDocument(target.path, loaded.document);
  try {
    await mkdir(dirname(target.path), { recursive: true });
    await writeFileAtomic(target.path, encoded);
  } catch (error) {
    throw createError('scope.mutation_write_failed', 'mutation write failed', undefined, error);
  }
  let workspace: Workspace;
  try { workspace = await reload(); }
  catch (error) {
    const rollbackErrors: unknown[] = [];
    try {
      if (loaded.existed && loaded.original) await writeFileAtomic(target.path, loaded.original);
      else await rm(target.path, { force: true });
    } catch (rollbackError) {
      rollbackErrors.push(rollbackError);
    }
    try {
      await reload();
    } catch (reloadError) {
      rollbackErrors.push(reloadError);
    }
    if (rollbackErrors.length > 0) {
      throw createError(
        'scope.mutation_write_failed',
        'mutation rollback failed',
        undefined,
        new AggregateError([error, ...rollbackErrors], 'mutation rollback failed'),
      );
    }
    if (error instanceof ApplicationError && error.code === 'invalid_data') throw mutationInvalidData('mutation produced invalid scope data', error);
    if (error instanceof ApplicationError && error.code === 'conflict') throw createError('scope.mutation_conflict', 'mutation conflicts with existing scope data', undefined, error);
    throw createError('scope.mutation_write_failed', 'mutation reload failed', undefined, error);
  }
  const updated = new ApplicationService(workspace);
  const result = mutationResult(updated, request, object, target.path);
  return { result, workspace };
};

interface MutationTarget { path: string; localId: string; relationIndex: number }
async function mutationTarget(service: ApplicationService, rootPath: string, request: MutationRequest): Promise<MutationTarget> {
  if (request.action === 'add') {
    let path = request.file || 'scope.locus.yaml';
    const normalized = path.replaceAll('\\', '/');
    if (!/\.locus\.(?:yaml|yml|json)$/u.test(normalized)) throw mutationInvalid('--file must name a lowercase *.locus.yaml definition');
    path = isAbsolute(path) ? resolve(path) : resolve(rootPath, path);
    await ensureMutationPath(rootPath, path);
    await rejectNestedScope(rootPath, path);
    return { path, localId: '', relationIndex: -1 };
  }
  if (request.kind === 'entity') {
    if (!request.ref) throw mutationInvalid('entity reference is required');
    let key: EntityKey;
    try { key = service.workspace.resolve(service.workspace.root, request.ref); }
    catch (error) { throw mutationNotFound(error instanceof Error ? error.message : String(error), error); }
    if (key.scope !== service.workspace.root) throw createError('scope.object_read_only', 'dependency entity is read-only', { reference: request.ref });
    const entity = service.workspace.scopes.get(key.scope)?.entities.get(key.id);
    if (!entity) throw mutationNotFound(`entity ${JSON.stringify(request.ref)} not found`);
    await ensureMutationPath(rootPath, entity.source.path);
    return { path: entity.source.path, localId: entity.localId, relationIndex: -1 };
  }
  if (!request.from || !request.to || request.type === undefined) throw mutationInvalid('relation reference is required');
  let from: EntityKey;
  let to: EntityKey;
  try {
    from = service.workspace.resolve(service.workspace.root, request.from);
    to = service.workspace.resolve(service.workspace.root, request.to);
  } catch (error) {
    throw mutationNotFound(error instanceof Error ? error.message : String(error), error);
  }
  for (const relation of service.workspace.relations) {
    if (sameKey(relation.from, from) && relation.type === request.type && sameKey(relation.to, to)) {
      if (relation.source.scope !== service.workspace.root) throw createError('scope.object_read_only', 'dependency relation is read-only');
      await ensureMutationPath(rootPath, relation.source.path);
      return { path: relation.source.path, localId: '', relationIndex: (relation.source.index ?? 1) - 1 };
    }
  }
  throw mutationNotFound(`relation ${request.from} ${request.type} ${request.to} not found`);
}

async function ensureMutationPath(root: string, target: string): Promise<void> {
  const rootAbsolute = resolve(root); const targetAbsolute = resolve(target);
  const rootReal = await realpath(rootAbsolute).catch(error => { throw sourceError(error, 'mutation root is unavailable'); });
  let probe = targetAbsolute; const suffix: string[] = [];
  for (;;) {
    try { await lstat(probe); break; }
    catch (error) {
      if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw sourceError(error, 'mutation path is unavailable');
      const parent = dirname(probe);
      if (parent === probe) throw mutationInvalid('definition path must remain inside root Scope');
      suffix.push(probe.slice(parent.length + (parent.endsWith(sep) ? 0 : 1))); probe = parent;
    }
  }
  let candidate = await realpath(probe).catch(error => { throw sourceError(error, 'mutation path is unavailable'); });
  for (let index = suffix.length - 1; index >= 0; index -= 1) candidate = join(candidate, suffix[index] as string);
  const rel = relative(rootReal, candidate);
  if (rel === '..' || rel.startsWith(`..${sep}`) || isAbsolute(rel)) throw mutationInvalid('definition path must remain inside root Scope');
}

async function rejectNestedScope(root: string, target: string): Promise<void> {
  const rootAbsolute = resolve(root); let directory = dirname(resolve(target));
  while (directory !== rootAbsolute) {
    for (const name of ['locus.yaml', 'locus.yml', 'locus.json']) {
      let exists = false;
      try { await lstat(join(directory, name)); exists = true; }
      catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw sourceError(error, 'inspect nested Scope failed'); }
      if (exists) throw mutationInvalid('definition path enters nested Scope');
    }
    const parent = dirname(directory);
    if (parent === directory || directory.length < rootAbsolute.length) throw mutationInvalid('definition path must remain inside root Scope');
    directory = parent;
  }
}

async function readMutableDocument(path: string): Promise<{ document: MutableDocument; original?: Buffer; existed: boolean }> {
  let data: Buffer;
  try { data = await readFile(path); }
  catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') return { document: { entities: [], relations: [] }, existed: false };
    throw sourceError(error, 'read definition failed');
  }
  let parsed: unknown;
  try { parsed = path.endsWith('.json') ? JSON.parse(data.toString('utf8')) : YAML.parse(data.toString('utf8')); }
  catch (error) { throw mutationInvalidData('decode definition failed', error); }
  if (parsed !== undefined && (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed))) throw mutationInvalidData('decode definition failed');
  const raw = (parsed ?? {}) as Record<string, unknown>;
  const document: MutableDocument = { entities: arrayObjects(raw.entities, 'entities'), relations: arrayObjects(raw.relations, 'relations') };
  if (typeof raw.group === 'string') document.group = raw.group;
  return { document, original: data, existed: true };
}
function arrayObjects(value: unknown, field: string): Record<string, unknown>[] {
  if (value === undefined || value === null) return [];
  if (!Array.isArray(value) || value.some(item => item === null || typeof item !== 'object' || Array.isArray(item))) throw mutationInvalidData(`invalid ${field} declaration`);
  return value as Record<string, unknown>[];
}
function encodeMutableDocument(path: string, document: MutableDocument): string {
  const value: Record<string, unknown> = { ...(document.group ? { group: document.group } : {}), ...(document.entities.length ? { entities: document.entities } : {}), ...(document.relations.length ? { relations: document.relations } : {}) };
  return path.endsWith('.json') ? `${JSON.stringify(value, null, 2)}\n` : YAML.stringify(value);
}

function mutateEntity(document: MutableDocument, request: MutationRequest, localID: string): Record<string, unknown> {
  if (request.action === 'add') {
    const object = cloneObject(request.object ?? {});
    applyFields(object, request.fields ?? {});
    const id = object.id;
    if (typeof id !== 'string' || !id.trim()) throw mutationInvalid('entity id is required');
    const local = localEntityID(id, document.group);
    if (document.entities.some(item => item.id === local)) throw createError('scope.entity_duplicate', 'entity already exists', { reference: id });
    object.id = local;
    document.entities.push(object);
    const result = cloneObject(object);
    result.id = id;
    return result;
  }
  const index = document.entities.findIndex(item => item.id === localID);
  if (index < 0) throw mutationNotFound('entity declaration not found');
  if (request.action === 'remove') return document.entities.splice(index, 1)[0] as Record<string, unknown>;
  const object = document.entities[index] as Record<string, unknown>;
  applyObjectMutation(object, request, { id: true });
  return cloneObject(object);
}
function mutateRelation(document: MutableDocument, request: MutationRequest, index: number): Record<string, unknown> {
  if (request.action === 'add') {
    const object = cloneObject(request.object ?? {});
    applyFields(object, request.fields ?? {});
    for (const field of ['from', 'type', 'to']) if (typeof object[field] !== 'string' || !(object[field] as string).trim()) throw mutationInvalid(`relation ${field} is required`);
    if (document.relations.some(item => item.from === object.from && item.type === object.type && item.to === object.to)) throw createError('scope.relation_duplicate', 'relation already exists');
    document.relations.push(object);
    return cloneObject(object);
  }
  if (index < 0 || index >= document.relations.length) throw mutationNotFound('relation declaration not found');
  if (request.action === 'remove') return document.relations.splice(index, 1)[0] as Record<string, unknown>;
  const object = document.relations[index] as Record<string, unknown>;
  applyObjectMutation(object, request, { from: true, type: true, to: true });
  return cloneObject(object);
}
function applyObjectMutation(object: Record<string, unknown>, request: MutationRequest, protectedFields: Readonly<Record<string, true>>): void {
  if (request.action === 'set') {
    const patch = cloneObject(request.object ?? {});
    for (const field of Object.keys(protectedFields)) delete patch[field];
    mergeMap(object, patch);
    applyFields(object, request.fields ?? {}, protectedFields);
    return;
  }
  if (request.action === 'unset') {
    for (const field of request.unset ?? []) {
      if (protectedFields[field]) throw createError('scope.field_invalid', 'structural field cannot be changed', { field });
      unsetPath(object, field);
    }
    return;
  }
  throw mutationInvalid(`unknown mutation action ${JSON.stringify(request.action)}`);
}
function applyFields(object: Record<string, unknown>, fields: Record<string, unknown>, protectedFields?: Readonly<Record<string, true>>): void {
  for (const [path, value] of Object.entries(fields)) {
    if (protectedFields?.[path]) throw createError('scope.field_invalid', 'structural field cannot be changed', { field: path });
    setPath(object, path, value);
  }
}
function mergeMap(target: Record<string, unknown>, patch: Record<string, unknown>): void {
  for (const [key, value] of Object.entries(patch)) {
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      const existing = target[key];
      const child = existing && typeof existing === 'object' && !Array.isArray(existing) ? existing as Record<string, unknown> : {};
      target[key] = child;
      mergeMap(child, value as Record<string, unknown>);
    } else target[key] = cloneValue(value);
  }
}
function setPath(object: Record<string, unknown>, fieldPath: string, value: unknown): void {
  const parts = fieldPath.split('.');
  if (!fieldPath || parts.some(part => !part)) throw createError('scope.field_invalid', 'invalid field path', { field: fieldPath });
  let current = object;
  for (const part of parts.slice(0, -1)) {
    const existing = current[part];
    if (existing === undefined) {
      current[part] = {};
      current = current[part] as Record<string, unknown>;
    } else if (existing && typeof existing === 'object' && !Array.isArray(existing)) current = existing as Record<string, unknown>;
    else throw createError('scope.field_invalid', 'field path crosses non-object', { field: fieldPath });
  }
  current[parts[parts.length - 1] as string] = cloneValue(value);
}
function unsetPath(object: Record<string, unknown>, fieldPath: string): void {
  const parts = fieldPath.split('.');
  if (!fieldPath || parts.some(part => !part)) throw createError('scope.field_invalid', 'invalid field path', { field: fieldPath });
  let current = object;
  for (const part of parts.slice(0, -1)) {
    const existing = current[part];
    if (!existing || typeof existing !== 'object' || Array.isArray(existing)) throw createError('scope.field_invalid', 'field does not exist', { field: fieldPath });
    current = existing as Record<string, unknown>;
  }
  const leaf = parts[parts.length - 1] as string;
  if (!(leaf in current)) throw createError('scope.field_invalid', 'field does not exist', { field: fieldPath });
  delete current[leaf];
}
function localEntityID(id: string, group: string | undefined): string {
  if (!group) {
    if (id.includes('/')) throw mutationInvalid('entity id does not belong to root group');
    return id;
  }
  const prefix = `${group}/`;
  if (!id.startsWith(prefix) || id.slice(prefix.length) === '') throw mutationInvalid('entity id does not belong to group');
  return id.slice(prefix.length);
}
function mutationResult(service: ApplicationService, request: MutationRequest, object: Record<string, unknown>, path: string): MutationResult {
  if (request.kind === 'entity') {
    const ref = request.action === 'add' ? String(object.id ?? '') : request.ref ?? '';
    try { const item = service.getEntity(ref, true); if (item.source) return { action: request.action, object: item, source: item.source }; }
    catch { /* removed entities have no reloaded source */ }
  } else if (request.from && request.to && request.type !== undefined) {
    try {
      const from = service.workspace.resolve(service.workspace.root, request.from); const to = service.workspace.resolve(service.workspace.root, request.to);
      const item = service.queryRelations([], true).find(value => sameKey(value.fromKey, from) && sameKey(value.toKey, to) && value.object.type === request.type);
      if (item?.source) return { action: request.action, object: item, source: item.source };
    } catch { /* use definition source below */ }
  }
  const root = service.workspace.scopes.get(service.workspace.root);
  const file = root ? relative(root.localPath, path).replaceAll('\\', '/') : '';
  return { action: request.action, object, source: { scope: service.workspace.root, scopeId: root?.manifest.id ?? '', file } };
}

declare module './service.js' { interface ApplicationService { mutate(request: MutationRequest, reload: WorkspaceReloader): Promise<{ result: MutationResult; workspace: Workspace }> } }
