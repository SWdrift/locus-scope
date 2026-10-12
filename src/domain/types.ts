import { resolve } from './resolve.js';
export type ScopeKey = string;

export interface EntityKey {
  readonly scope: ScopeKey;
  readonly id: string;
}

export interface Provenance {
  readonly scope: ScopeKey;
  readonly file: string;
  readonly group?: string;
  readonly line?: number;
  readonly index?: number;
  readonly path: string;
}

export interface Entity {
  readonly id: string;
  readonly properties: Record<string, unknown>;
  readonly source: Provenance;
  readonly localId: string;
}

export interface Manifest {
  readonly id: string;
  readonly imports: Record<string, string>;
  readonly exports: string[];
}

export interface Relation {
  readonly from: EntityKey;
  readonly type: string;
  readonly to: EntityKey;
  readonly fromRef: string;
  readonly toRef: string;
  readonly properties: Record<string, unknown>;
  readonly source: Provenance;
}

export interface Source {
  readonly key: ScopeKey;
  readonly localPath: string;
}

export interface Resolver {
  resolve(from: Source, reference: string): Promise<Source> | Source;
}

export class Scope {
  readonly key: ScopeKey;
  readonly manifest: Manifest;
  readonly entities: Map<string, Entity>;
  readonly imports: Map<string, ScopeKey>;
  readonly localPath: string;
  readonly manifestFile: string;
  /** Internal declaration data used while validating relations. */
  readonly relationDecls: RelationDeclaration[] = [];
  /** Internal export set used by projection resolution. */
  readonly exported: Set<string>;

  constructor(source: Source, manifest: Manifest, manifestFile: string) {
    this.key = source.key;
    this.localPath = source.localPath;
    this.manifest = manifest;
    this.manifestFile = manifestFile;
    this.entities = new Map();
    this.imports = new Map();
    this.exported = new Set(manifest.exports);
  }
}

export interface RelationDeclaration {
  readonly from: string;
  readonly type: string;
  readonly to: string;
  readonly properties: Record<string, unknown>;
  readonly source: Provenance;
}

export class Workspace {
  readonly root: ScopeKey;
  readonly scopes: Map<ScopeKey, Scope>;
  relations: Relation[];

  constructor(root: ScopeKey, scopes = new Map<ScopeKey, Scope>()) {
    this.root = root;
    this.scopes = scopes;
    this.relations = [];
  }

  resolve(from: ScopeKey, ref: string): EntityKey {
    return resolve(this, from, ref);
  }
}

