import path from 'node:path';
import { resolve } from './resolve.js';
import type { EntityKey, Relation, RelationDeclaration, Scope, Workspace } from './types.js';

export function compareRelations(left: Relation, right: Relation): number {
  const values: [string, string][] = [
    [left.from.scope, right.from.scope], [left.from.id, right.from.id], [left.type, right.type],
    [left.to.scope, right.to.scope], [left.to.id, right.to.id],
  ];
  for (const [a, b] of values) { if (a < b) return -1; if (a > b) return 1; }
  return 0;
}

export function validateWorkspace(workspace: Workspace): void {
  const keys = [...workspace.scopes.keys()].sort();
  for (const key of keys) {
    const scope = workspace.scopes.get(key) as Scope;
    for (const reference of scope.manifest.exports) {
      try { resolve(workspace, key, reference); }
      catch (error) { throw new Error(`${path.join(scope.localPath, scope.manifestFile)}: export ${JSON.stringify(reference)}: ${(error as Error).message}`, { cause: error }); }
    }
  }
  const identities = new Map<string, RelationDeclaration['source']>();
  const relations: Relation[] = [];
  for (const key of keys) {
    const scope = workspace.scopes.get(key) as Scope;
    for (const declaration of scope.relationDecls) {
      let from: EntityKey;
      try { from = resolve(workspace, key, declaration.from); }
      catch (error) { throw new Error(`${declaration.source.path}:${declaration.source.line ?? 0}: relation ${JSON.stringify(relationText(declaration))}: start reference ${JSON.stringify(declaration.from)}: ${(error as Error).message}`, { cause: error }); }
      let to: EntityKey;
      try { to = resolve(workspace, key, declaration.to); }
      catch (error) { throw new Error(`${declaration.source.path}:${declaration.source.line ?? 0}: relation ${JSON.stringify(relationText(declaration))}: end reference ${JSON.stringify(declaration.to)}: ${(error as Error).message}`, { cause: error }); }
      const identity = `${from.scope}\0${from.id}\0${declaration.type}\0${to.scope}\0${to.id}`;
      const previous = identities.get(identity);
      if (previous) throw new Error(`${declaration.source.path}:${declaration.source.line ?? 0}: relation ${JSON.stringify(relationText(declaration))} duplicates relation declared in ${previous.path}:${previous.line ?? 0}`);
      identities.set(identity, declaration.source);
      relations.push({ from, type: declaration.type, to, fromRef: declaration.from, toRef: declaration.to, properties: declaration.properties, source: declaration.source });
    }
  }
  relations.sort(compareRelations);
  workspace.relations = relations;
}

function relationText(declaration: RelationDeclaration): string { return `${declaration.from} ${declaration.type} ${declaration.to}`; }
export const Validate = validateWorkspace;
export const CompareRelations = compareRelations;
