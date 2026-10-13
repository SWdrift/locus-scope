import { Match } from "../domain/index.js";
import { createError } from "../domain/errors.js";
import type { EntityKey, Predicate, Relation as DomainRelation } from "../domain/index.js";
import type { Entity, GraphResult, ImpactResult, PathResult, Predicates, Relation } from "./types.js";
import { ApplicationService, compareEntityKey, entityKeyId } from "./service.js";

interface Adjacency {
  readonly outgoing: Map<string, number[]>;
  readonly incoming: Map<string, number[]>;
}

function makeAdjacency(relations: readonly DomainRelation[]): Adjacency {
  const outgoing = new Map<string, number[]>();
  const incoming = new Map<string, number[]>();
  relations.forEach((relation, index) => {
    const from = entityKeyId(relation.from);
    const to = entityKeyId(relation.to);
    const out = outgoing.get(from) ?? [];
    out.push(index);
    outgoing.set(from, out);
    const into = incoming.get(to) ?? [];
    into.push(index);
    incoming.set(to, into);
  });
  return { outgoing, incoming };
}

function sortedKeys(set: Set<string>, values: Map<string, EntityKey>): EntityKey[] {
  return [...set]
    .map(key => values.get(key))
    .filter((key): key is EntityKey => key !== undefined)
    .sort(compareEntityKey);
}

ApplicationService.prototype.graph = function graph(
  this: ApplicationService,
  seeds: EntityKey[],
  depth: number,
  via: Predicates = [],
  withSource = false
): GraphResult {
  if (depth < 0) throw createError("scope.selector_invalid", "graph depth must be non-negative");
  const adjacency = makeAdjacency(this.workspace.relations);
  const levels = new Map<string, number>();
  const keyValues = new Map<string, EntityKey>();
  const queue: EntityKey[] = [];
  for (const seed of seeds) {
    this.entityByKey(seed, false);
    const id = entityKeyId(seed);
    keyValues.set(id, seed);
    if (!levels.has(id)) {
      levels.set(id, 0);
      queue.push(seed);
    }
  }
  const selected = new Set<number>();
  while (queue.length) {
    const current = queue.shift();
    if (!current) break;
    const currentId = entityKeyId(current);
    const level = levels.get(currentId) ?? 0;
    if (level >= depth) continue;
    for (const index of adjacency.outgoing.get(currentId) ?? []) {
      const relation = this.workspace.relations[index];
      if (!relation || !relationMatches(this, relation, via)) continue;
      selected.add(index);
      const targetId = entityKeyId(relation.to);
      keyValues.set(targetId, relation.to);
      if (!levels.has(targetId)) {
        levels.set(targetId, level + 1);
        queue.push(relation.to);
      }
    }
  }
  const seedSet = new Set(seeds.map(entityKeyId));
  const nodes = this.entitiesByKeys(sortedKeys(new Set(levels.keys()), keyValues), withSource);
  return {
    seeds: this.entitiesByKeys(sortedKeys(seedSet, keyValues), withSource),
    depth,
    nodes,
    relations: this.relationsByIndexes(
      [...selected].sort((a, b) => a - b),
      withSource
    )
  };
};

ApplicationService.prototype.path = function path(
  this: ApplicationService,
  from: EntityKey,
  to: EntityKey,
  via: Predicates = [],
  withSource = false
): PathResult {
  this.entityByKey(from, false);
  this.entityByKey(to, false);
  const adjacency = makeAdjacency(this.workspace.relations);
  const seen = new Set<string>([entityKeyId(from)]);
  const previous = new Map<string, { key: EntityKey; relation: number }>();
  const queue: EntityKey[] = [from];
  while (queue.length) {
    const current = queue.shift();
    if (!current) break;
    if (current.scope === to.scope && current.id === to.id) break;
    for (const index of adjacency.outgoing.get(entityKeyId(current)) ?? []) {
      const relation = this.workspace.relations[index];
      if (!relation || !relationMatches(this, relation, via)) continue;
      const targetId = entityKeyId(relation.to);
      if (seen.has(targetId)) continue;
      seen.add(targetId);
      previous.set(targetId, { key: current, relation: index });
      queue.push(relation.to);
    }
  }
  const targetId = entityKeyId(to);
  if (!seen.has(targetId)) throw createError("scope.path_unreachable", "no directed path between entities");
  const keys: EntityKey[] = [to];
  const indexes: number[] = [];
  let cursor = to;
  while (cursor.scope !== from.scope || cursor.id !== from.id) {
    const prior = previous.get(entityKeyId(cursor));
    if (!prior) throw createError("scope.path_unreachable", "no directed path between entities");
    indexes.push(prior.relation);
    cursor = prior.key;
    keys.push(cursor);
  }
  keys.reverse();
  indexes.reverse();
  const nodes = this.entitiesByKeys(keys, withSource);
  return {
    from: nodes[0] as Entity,
    to: nodes[nodes.length - 1] as Entity,
    nodes,
    relations: this.relationsByIndexes(indexes, withSource)
  };
};

ApplicationService.prototype.impact = function impact(
  this: ApplicationService,
  seeds: EntityKey[],
  via: Predicates = [],
  withSource = false
): ImpactResult {
  const adjacency = makeAdjacency(this.workspace.relations);
  const seen = new Set<string>();
  const seedSet = new Set<string>();
  const keyValues = new Map<string, EntityKey>();
  const queue: EntityKey[] = [];
  for (const seed of seeds) {
    this.entityByKey(seed, false);
    const id = entityKeyId(seed);
    keyValues.set(id, seed);
    if (!seen.has(id)) {
      seen.add(id);
      seedSet.add(id);
      queue.push(seed);
    }
  }
  const selected = new Set<number>();
  while (queue.length) {
    const current = queue.shift();
    if (!current) break;
    for (const index of adjacency.incoming.get(entityKeyId(current)) ?? []) {
      const relation = this.workspace.relations[index];
      if (!relation || !relationMatches(this, relation, via)) continue;
      selected.add(index);
      const sourceId = entityKeyId(relation.from);
      keyValues.set(sourceId, relation.from);
      if (!seen.has(sourceId)) {
        seen.add(sourceId);
        queue.push(relation.from);
      }
    }
  }
  const seedItems = this.entitiesByKeys(sortedKeys(seedSet, keyValues), withSource);
  const affectedItems = this.entitiesByKeys(
    sortedKeys(new Set([...seen].filter(id => !seedSet.has(id))), keyValues),
    withSource
  );
  return {
    seeds: seedItems,
    affected: affectedItems,
    relations: this.relationsByIndexes(
      [...selected].sort((a, b) => a - b),
      withSource
    )
  };
};

ApplicationService.prototype.entitiesByKeys = function entitiesByKeys(
  this: ApplicationService,
  keys: EntityKey[],
  withSource: boolean
): Entity[] {
  return keys.map(key => this.entityByKey(key, withSource));
};
ApplicationService.prototype.relationsByIndexes = function relationsByIndexes(
  this: ApplicationService,
  indexes: number[],
  withSource: boolean
): Relation[] {
  const all = this.queryRelations([], withSource);
  return indexes.map(index => all[index]).filter((item): item is Relation => item !== undefined);
};

function relationMatches(service: ApplicationService, relation: DomainRelation, predicates: Predicates): boolean {
  const object = { ...relation.properties, from: relation.fromRef, type: relation.type, to: relation.toRef };
  const metadata = {
    scope: service.scopeReference(relation.source.scope),
    fromScope: service.scopeReference(relation.from.scope),
    toScope: service.scopeReference(relation.to.scope)
  };
  return Match(object, metadata, predicates as Predicate[]);
}

declare module "./service.js" {
  interface ApplicationService {
    graph(seeds: EntityKey[], depth: number, via?: Predicates, withSource?: boolean): GraphResult;
    path(from: EntityKey, to: EntityKey, via?: Predicates, withSource?: boolean): PathResult;
    impact(seeds: EntityKey[], via?: Predicates, withSource?: boolean): ImpactResult;
    entitiesByKeys(keys: EntityKey[], withSource: boolean): Entity[];
    relationsByIndexes(indexes: number[], withSource: boolean): Relation[];
  }
}
