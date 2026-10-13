import { ApplicationService, compareEntityKey, entityKeyId } from './service.js';
import type { EntityKey } from '../domain/index.js';
import type { DiffResult, Entity, Relation, WorkspaceLoader } from './types.js';
import { invalidArgument } from '../domain/errors.js';

interface RelationIdentity { from: EntityKey; type: string; to: EntityKey }
interface SemanticGraph { entities: Map<string, Entity>; relations: Map<string, Relation> }

function relationIdentityId(value: RelationIdentity): string { return JSON.stringify([value.from.scope, value.from.id, value.type, value.to.scope, value.to.id]); }
function relationSort(left: RelationIdentity, right: RelationIdentity): number {
  return compareEntityKey(left.from, right.from) || (left.type < right.type ? -1 : left.type > right.type ? 1 : 0) || compareEntityKey(left.to, right.to);
}
function equalValue(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true;
  if (typeof left !== typeof right || left === null || right === null) return false;
  if (Array.isArray(left) && Array.isArray(right)) return left.length === right.length && left.every((value, index) => equalValue(value, right[index]));
  if (typeof left === 'object' && typeof right === 'object') {
    const a = Object.entries(left as Record<string, unknown>); const b = Object.entries(right as Record<string, unknown>);
    return a.length === b.length && a.every(([key, value]) => key in (right as Record<string, unknown>) && equalValue(value, (right as Record<string, unknown>)[key]));
  }
  return false;
}

ApplicationService.prototype.diff = async function diff(this: ApplicationService, left: string, right: string, loadPath: WorkspaceLoader): Promise<DiffResult> {
  const leftGraph = await selectGraph(this, left, loadPath); const rightGraph = await selectGraph(this, right, loadPath);
  const result: DiffResult = { added: { entities: [], relations: [] }, removed: { entities: [], relations: [] }, changed: { entities: [], relations: [] } };
  const entityKeys = [...leftGraph.entities.keys()].map(value => JSON.parse(value) as [string, string]).map(([scope, id]) => ({ scope, id })).sort(compareEntityKey);
  for (const key of entityKeys) {
    const id = entityKeyId(key); const before = leftGraph.entities.get(id); const after = rightGraph.entities.get(id);
    if (!before) continue;
    if (!after) result.removed.entities.push(before); else if (!equalValue(before.object, after.object)) result.changed.entities.push({ before, after });
  }
  const rightEntityKeys = [...rightGraph.entities.keys()].map(value => JSON.parse(value) as [string, string]).map(([scope, id]) => ({ scope, id })).sort(compareEntityKey);
  for (const key of rightEntityKeys) if (!leftGraph.entities.has(entityKeyId(key))) {
    const item = rightGraph.entities.get(entityKeyId(key)); if (item) result.added.entities.push(item);
  }
  const relationKeys = [...leftGraph.relations.values()].map(relation => ({ from: relation.fromKey, type: String(relation.object.type ?? ''), to: relation.toKey })).sort(relationSort);
  for (const identity of relationKeys) {
    const id = relationIdentityId(identity); const before = leftGraph.relations.get(id); const after = rightGraph.relations.get(id);
    if (!before) continue;
    if (!after) result.removed.relations.push(before); else if (!equalValue(before.object, after.object)) result.changed.relations.push({ before, after });
  }
  const rightRelations = [...rightGraph.relations.values()].map(relation => ({ identity: { from: relation.fromKey, type: String(relation.object.type ?? ''), to: relation.toKey }, relation })).sort((a, b) => relationSort(a.identity, b.identity));
  for (const item of rightRelations) if (!leftGraph.relations.has(relationIdentityId(item.identity))) result.added.relations.push(item.relation);
  return result;
};

async function selectGraph(service: ApplicationService, selector: string, loadPath: WorkspaceLoader): Promise<SemanticGraph> {
  const separator = selector.indexOf(':');
  if (separator <= 0 || separator === selector.length - 1) throw invalidArgument('scope.diff_input_invalid', 'invalid diff selector');
  const kind = selector.slice(0, separator); const value = selector.slice(separator + 1);
  if (kind === 'scope') return graphForScope(service, service.resolveScope(value), '');
  if (kind === 'group') {
    const hash = value.lastIndexOf('#'); if (hash <= 0 || hash === value.length - 1) throw invalidArgument('scope.diff_input_invalid', 'invalid group diff selector');
    return graphForScope(service, service.resolveScope(value.slice(0, hash)), value.slice(hash + 1));
  }
  if (kind === 'path') return graphForWorkspace(new ApplicationService(await loadPath(value)));
  throw invalidArgument('scope.diff_input_invalid', 'unknown diff selector kind');
}

function graphForScope(service: ApplicationService, owner: string, group: string): SemanticGraph {
  const selected = new Set<string>(); const scope = service.workspace.scopes.get(owner);
  if (!scope) throw new Error('scope not found');
  for (const [id, entity] of scope.entities) if (!group || entity.source.group === group) selected.add(entityKeyId({ scope: owner, id }));
  return graphForKeys(service, selected);
}
function graphForWorkspace(service: ApplicationService): SemanticGraph {
  const selected = new Set<string>();
  for (const [scopeKey, scope] of service.workspace.scopes) for (const id of scope.entities.keys()) selected.add(entityKeyId({ scope: scopeKey, id }));
  return graphForKeys(service, selected);
}
function graphForKeys(service: ApplicationService, selected: Set<string>): SemanticGraph {
  const entities = new Map<string, Entity>(); const relations = new Map<string, Relation>();
  for (const id of selected) {
    const key = JSON.parse(id) as [string, string]; entities.set(id, service.entityByKey({ scope: key[0] as string, id: key[1] as string }, false));
  }
  for (const relation of service.queryRelations([], false)) {
    if (!selected.has(entityKeyId(relation.fromKey)) || !selected.has(entityKeyId(relation.toKey))) continue;
    const identity = relationIdentityId({ from: relation.fromKey, type: String(relation.object.type ?? ''), to: relation.toKey }); relations.set(identity, relation);
  }
  return { entities, relations };
}

declare module './service.js' { interface ApplicationService { diff(left: string, right: string, loadPath: WorkspaceLoader): Promise<DiffResult> } }
