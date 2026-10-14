import { Match } from "../domain/index.js";
import { createError } from "../domain/errors.js";
import type { EntityKey, Predicate, Provenance, ScopeKey, Workspace } from "../domain/index.js";
import type { Entity, Group, Relation, Scope, Source, ValidationResult, Predicates } from "./types.js";

export function cloneValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(cloneValue);
  if (value !== null && typeof value === "object") {
    const result: Record<string, unknown> = {};
    for (const [key, child] of Object.entries(value))
      Object.defineProperty(result, key, {
        value: cloneValue(child),
        enumerable: true,
        writable: true,
        configurable: true
      });
    return result;
  }
  return value;
}

export function cloneObject(value: Record<string, unknown>): Record<string, unknown> {
  return cloneValue(value) as Record<string, unknown>;
}

export function compareEntityKey(left: EntityKey, right: EntityKey): number {
  return left.scope < right.scope
    ? -1
    : left.scope > right.scope
      ? 1
      : left.id < right.id
        ? -1
        : left.id > right.id
          ? 1
          : 0;
}

export function entityKeyId(key: EntityKey): string {
  return JSON.stringify([key.scope, key.id]);
}

export class ApplicationService {
  readonly workspace: Workspace;
  private readonly scopeRefs: Map<ScopeKey, string>;
  private readonly entityRefs: Map<string, string>;

  constructor(workspace: Workspace) {
    this.workspace = workspace;
    this.scopeRefs = makeScopeRefs(workspace);
    this.entityRefs = makeEntityRefs(workspace, this.scopeRefs);
  }

  getWorkspace(): Workspace {
    return this.workspace;
  }

  validate(): ValidationResult {
    let entities = 0;
    for (const scope of this.workspace.scopes.values()) entities += scope.entities.size;
    return {
      valid: true,
      root: this.workspace.root,
      scopes: this.workspace.scopes.size,
      entities,
      relations: this.workspace.relations.length
    };
  }

  queryScopes(predicates: Predicates = [], withSource = false): Scope[] {
    const result: Scope[] = [];
    for (const key of [...this.workspace.scopes.keys()].sort()) {
      const current = this.workspace.scopes.get(key);
      if (!current) continue;
      const object: Record<string, unknown> = {
        id: current.manifest.id,
        imports: { ...current.manifest.imports },
        exports: [...current.manifest.exports],
        root: key === this.workspace.root
      };
      const metadata = { scope: this.scopeRefs.get(key) ?? "" };
      if (!Match(object, metadata, predicates as Predicate[])) continue;
      const item: { key: ScopeKey; ref: string; object: Record<string, unknown>; source?: Source } = {
        key,
        ref: this.scopeRefs.get(key) ?? "",
        object
      };
      if (withSource) item.source = { scope: key, scopeId: current.manifest.id, file: current.manifestFile };
      result.push(item);
    }
    return result;
  }

  getScope(reference: string, withSource = false): Scope {
    const key = this.resolveScope(reference);
    const item = this.queryScopes([], withSource).find(candidate => candidate.key === key);
    if (!item) throw createError("scope.object_not_found", `scope ${reference} not found`, { reference });
    return item;
  }

  queryGroups(predicates: Predicates = [], withSource = false): Group[] {
    const groups = new Map<string, { scope: ScopeKey; group: string; sources: Source[] }>();
    for (const [scopeKey, current] of this.workspace.scopes) {
      for (const entity of current.entities.values()) {
        const group = entity.source.group;
        if (!group) continue;
        const id = JSON.stringify([scopeKey, group]);
        const existing = groups.get(id) ?? { scope: scopeKey, group, sources: [] };
        const source = this.makeSource(entity.source);
        if (!existing.sources.some(value => value.file === source.file)) existing.sources.push(source);
        groups.set(id, existing);
      }
    }
    const values = [...groups.values()].sort((left, right) =>
      left.scope < right.scope
        ? -1
        : left.scope > right.scope
          ? 1
          : left.group < right.group
            ? -1
            : left.group > right.group
              ? 1
              : 0
    );
    const result: Group[] = [];
    for (const value of values) {
      const ref = `${this.scopeRefs.get(value.scope) ?? ""}#${value.group}`;
      const object = { id: value.group };
      if (
        !Match(object, { scope: this.scopeRefs.get(value.scope) ?? "", group: value.group }, predicates as Predicate[])
      )
        continue;
      const item: { scope: ScopeKey; ref: string; object: Record<string, unknown>; sources?: Source[] } = {
        scope: value.scope,
        ref,
        object
      };
      if (withSource) item.sources = value.sources.sort((a, b) => (a.file < b.file ? -1 : a.file > b.file ? 1 : 0));
      result.push(item);
    }
    return result;
  }

  getGroup(reference: string, withSource = false): Group {
    const separator = reference.lastIndexOf("#");
    if (separator <= 0 || separator === reference.length - 1)
      throw createError("scope.selector_invalid", "invalid group reference");
    const key = this.resolveScope(reference.slice(0, separator));
    const group = reference.slice(separator + 1);
    const item = this.queryGroups([], withSource).find(
      candidate => candidate.scope === key && candidate.object.id === group
    );
    if (!item) throw createError("scope.object_not_found", `group ${reference} not found`, { reference });
    return item;
  }

  queryEntities(predicates: Predicates = [], withSource = false): Entity[] {
    const keys: EntityKey[] = [];
    for (const [scope, current] of this.workspace.scopes)
      for (const id of current.entities.keys()) keys.push({ scope, id });
    keys.sort(compareEntityKey);
    const result: Entity[] = [];
    for (const key of keys) {
      const entity = this.workspace.scopes.get(key.scope)?.entities.get(key.id);
      if (!entity) continue;
      const object = cloneObject(entity.properties);
      object.id = entity.id;
      if (
        !Match(
          object,
          { scope: this.scopeRefs.get(key.scope) ?? "", group: entity.source.group ?? "" },
          predicates as Predicate[]
        )
      )
        continue;
      const item: { key: EntityKey; ref?: string; object: Record<string, unknown>; source?: Source } = { key, object };
      const ref = this.entityRefs.get(entityKeyId(key));
      if (ref !== undefined) item.ref = ref;
      if (withSource) item.source = this.makeSource(entity.source);
      result.push(item);
    }
    return result;
  }

  getEntity(reference: string, withSource = false): Entity {
    const key = this.workspace.resolve(this.workspace.root, reference);
    return this.entityByKey(key, withSource);
  }

  entityByKey(key: EntityKey, withSource = false): Entity {
    const item = this.queryEntities([], withSource).find(
      candidate => candidate.key.scope === key.scope && candidate.key.id === key.id
    );
    if (!item) throw createError("scope.object_not_found", "entity not found", { reference: key.id });
    return item;
  }

  queryRelations(predicates: Predicates = [], withSource = false): Relation[] {
    const result: Relation[] = [];
    for (const relation of this.workspace.relations) {
      const object = cloneObject(relation.properties);
      object.from = relation.fromRef;
      object.type = relation.type;
      object.to = relation.toRef;
      if (
        !Match(
          object,
          {
            scope: this.scopeRefs.get(relation.source.scope) ?? "",
            fromScope: this.scopeRefs.get(relation.from.scope) ?? "",
            toScope: this.scopeRefs.get(relation.to.scope) ?? ""
          },
          predicates as Predicate[]
        )
      )
        continue;
      const item: { fromKey: EntityKey; toKey: EntityKey; object: Record<string, unknown>; source?: Source } = {
        fromKey: relation.from,
        toKey: relation.to,
        object
      };
      if (withSource) item.source = this.makeSource(relation.source);
      result.push(item);
    }
    return result;
  }

  resolveScope(reference: string): ScopeKey {
    if (reference === ".") return this.workspace.root;
    let current = this.workspace.root;
    for (const alias of reference.split(":")) {
      const next = this.workspace.scopes.get(current)?.imports.get(alias);
      if (!next)
        throw createError("scope.object_not_found", `scope reference ${reference} does not resolve at alias ${alias}`, {
          reference
        });
      current = next;
    }
    return current;
  }

  scopeReference(key: ScopeKey): string {
    return this.scopeRefs.get(key) ?? "";
  }
  makeSource(provenance: Provenance): Source {
    const scope = this.workspace.scopes.get(provenance.scope);
    if (!scope) throw createError("scope.internal", "source scope not found");
    const result: { scope: ScopeKey; scopeId: string; file: string; group?: string; line?: number; index?: number } = {
      scope: provenance.scope,
      scopeId: scope.manifest.id,
      file: provenance.file
    };
    if (provenance.group !== undefined && provenance.group !== "") result.group = provenance.group;
    if (provenance.line !== undefined && provenance.line !== 0) result.line = provenance.line;
    if (provenance.index !== undefined && provenance.index !== 0) result.index = provenance.index;
    return result;
  }
}

export function makeScopeRefs(workspace: Workspace): Map<ScopeKey, string> {
  const refs = new Map<ScopeKey, string>([[workspace.root, "."]]);
  const queue: ScopeKey[] = [workspace.root];
  while (queue.length) {
    const current = queue.shift();
    if (current === undefined) break;
    const scope = workspace.scopes.get(current);
    if (!scope) continue;
    for (const alias of [...scope.imports.keys()].sort()) {
      const target = scope.imports.get(alias);
      if (!target) continue;
      const candidate = refs.get(current) === "." ? alias : `${refs.get(current) ?? ""}:${alias}`;
      if (!refs.has(target) || candidate < (refs.get(target) ?? "")) {
        refs.set(target, candidate);
        queue.push(target);
      }
    }
  }
  return refs;
}

export function makeEntityRefs(workspace: Workspace, scopeRefs: Map<ScopeKey, string>): Map<string, string> {
  const refs = new Map<string, string>();
  const root = workspace.scopes.get(workspace.root);
  if (root) for (const id of root.entities.keys()) refs.set(entityKeyId({ scope: workspace.root, id }), id);
  for (const key of [...workspace.scopes.keys()].sort()) {
    if (key === workspace.root) continue;
    const scope = workspace.scopes.get(key);
    const prefix = scopeRefs.get(key);
    if (!scope || prefix === undefined) continue;
    for (const exported of scope.manifest.exports) {
      const candidate = `${prefix}:${exported}`;
      try {
        const resolved = workspace.resolve(workspace.root, candidate);
        const id = entityKeyId(resolved);
        if (!refs.has(id) || candidate < (refs.get(id) ?? "")) refs.set(id, candidate);
      } catch {
        /* invalid exports are reported by domain validation */
      }
    }
  }
  return refs;
}
