import type { EntityKey, Predicate, ScopeKey, Workspace } from '../domain/index.js';

export interface ValidationResult {
  readonly valid: boolean;
  readonly root: ScopeKey;
  readonly scopes: number;
  readonly entities: number;
  readonly relations: number;
}

export interface Source {
  readonly scope: ScopeKey;
  readonly scopeId: string;
  readonly file: string;
  readonly group?: string;
  readonly line?: number;
  readonly index?: number;
}

export interface Entity {
  readonly key: EntityKey;
  readonly ref?: string;
  readonly object: Record<string, unknown>;
  readonly source?: Source;
}

export interface Relation {
  readonly fromKey: EntityKey;
  readonly toKey: EntityKey;
  readonly object: Record<string, unknown>;
  readonly source?: Source;
}

export interface Scope {
  readonly key: ScopeKey;
  readonly ref: string;
  readonly object: Record<string, unknown>;
  readonly source?: Source;
}

export interface Group {
  readonly scope: ScopeKey;
  readonly ref: string;
  readonly object: Record<string, unknown>;
  readonly sources?: Source[];
}

export interface GraphResult {
  readonly seeds: Entity[];
  readonly depth: number;
  readonly nodes: Entity[];
  readonly relations: Relation[];
}

export interface PathResult {
  readonly from: Entity;
  readonly to: Entity;
  readonly nodes: Entity[];
  readonly relations: Relation[];
}

export interface ImpactResult {
  readonly seeds: Entity[];
  readonly affected: Entity[];
  readonly relations: Relation[];
}

export interface DiffSide {
  readonly entities: Entity[];
  readonly relations: Relation[];
}

export interface EntityChange {
  readonly before: Entity;
  readonly after: Entity;
}

export interface RelationChange {
  readonly before: Relation;
  readonly after: Relation;
}

export interface DiffResult {
  readonly added: DiffSide;
  readonly removed: DiffSide;
  readonly changed: { readonly entities: EntityChange[]; readonly relations: RelationChange[] };
}

export interface MutationRequest {
  readonly kind: 'entity' | 'relation' | string;
  readonly action: 'add' | 'remove' | 'set' | 'unset' | string;
  readonly ref?: string;
  readonly from?: string;
  readonly type?: string;
  readonly to?: string;
  readonly object?: Record<string, unknown>;
  readonly fields?: Record<string, unknown>;
  readonly unset?: string[];
  readonly file?: string;
}

export interface MutationResult {
  readonly action: string;
  readonly object: unknown;
  readonly source: Source;
}

export type WorkspaceLoader = (path: string) => Workspace | Promise<Workspace>;
export type WorkspaceReloader = () => Workspace | Promise<Workspace>;
export type Predicates = readonly Predicate[];
