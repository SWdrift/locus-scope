import type { EntityKey, Scope, ScopeKey, Workspace } from "./types.js";
import { createError } from "./errors.js";

export function resolve(workspace: Workspace, fromKey: ScopeKey, reference: string): EntityKey {
  const current = workspace.scopes.get(fromKey);
  if (!current) throw createError("scope.object_not_found", "Requested scope is not loaded");
  if (typeof reference !== "string" || !reference.trim())
    throw createError("scope.reference_unresolved", `${scopeLabel(current)}: entity reference is empty`);
  const separator = reference.indexOf(":");
  if (separator < 0) {
    if (!current.entities.has(reference))
      throw createError("scope.object_not_found", `${scopeLabel(current)} does not contain the requested entity`, {
        reference
      });
    return { scope: current.key, id: reference };
  }
  const projection = reference.slice(0, separator);
  const rest = reference.slice(separator + 1);
  if (!projection.trim() || !rest.trim() || reference.split(":").some(part => !part.trim()))
    throw createError("scope.reference_unresolved", `${scopeLabel(current)}: malformed projected reference`, {
      reference
    });
  const targetKey = current.imports.get(projection);
  if (!targetKey)
    throw createError("scope.reference_unresolved", `${scopeLabel(current)} does not import the requested projection`, {
      reference
    });
  const target = workspace.scopes.get(targetKey);
  if (!target)
    throw createError("scope.internal", `${scopeLabel(current)}: import projection targets an unloaded scope`);
  if (!target.exported.has(rest))
    throw createError("scope.object_not_found", `${scopeLabel(target)} does not export the requested entity`, {
      reference
    });
  return resolve(workspace, targetKey, rest);
}

export function scopeLabel(scope: Scope): string {
  return `scope ${JSON.stringify(scope.manifest.id)}`;
}

export const Resolve = resolve;
