import type { EntityKey, Scope, ScopeKey, Workspace } from './types.js';

export function resolve(workspace: Workspace, fromKey: ScopeKey, reference: string): EntityKey {
  const current = workspace.scopes.get(fromKey);
  if (!current) throw new Error(`scope source ${JSON.stringify(fromKey)} is not loaded`);
  if (!reference.trim()) throw new Error(`${scopeLabel(current)}: entity reference is empty`);
  const separator = reference.indexOf(':');
  if (separator < 0) {
    if (!current.entities.has(reference)) throw new Error(`${scopeLabel(current)} does not contain entity ${JSON.stringify(reference)}`);
    return { scope: current.key, id: reference };
  }
  const projection = reference.slice(0, separator);
  const rest = reference.slice(separator + 1);
  if (!projection || !rest) throw new Error(`${scopeLabel(current)}: malformed projected reference ${JSON.stringify(reference)}`);
  const targetKey = current.imports.get(projection);
  if (!targetKey) throw new Error(`${scopeLabel(current)} does not import projection ${JSON.stringify(projection)} referenced by ${JSON.stringify(reference)}`);
  const target = workspace.scopes.get(targetKey);
  if (!target) throw new Error(`${scopeLabel(current)}: projection ${JSON.stringify(projection)} targets unloaded scope ${JSON.stringify(targetKey)}`);
  if (!target.exported.has(rest)) throw new Error(`${scopeLabel(target)} does not export ${JSON.stringify(rest)} (required by projection ${JSON.stringify(projection)} from ${scopeLabel(current)})`);
  return resolve(workspace, targetKey, rest);
}

export function scopeLabel(scope: Scope): string {
  return `scope ${JSON.stringify(scope.manifest.id)} (${scope.key})`;
}

export const Resolve = resolve;
