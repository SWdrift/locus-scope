import { createError } from "./errors.js";
import { resolve } from "./resolve.js";
import type { EntityKey, Relation, RelationDeclaration, Scope, Workspace } from "./types.js";

export function compareRelations(left: Relation, right: Relation): number {
  const values: [string, string][] = [
    [left.from.scope, right.from.scope],
    [left.from.id, right.from.id],
    [left.type, right.type],
    [left.to.scope, right.to.scope],
    [left.to.id, right.to.id]
  ];
  for (const [a, b] of values) {
    if (a < b) return -1;
    if (a > b) return 1;
  }
  return 0;
}

export function validateWorkspace(workspace: Workspace): void {
  const keys = [...workspace.scopes.keys()].sort();
  for (const key of keys) {
    const scope = workspace.scopes.get(key) as Scope;
    for (const reference of scope.manifest.exports) {
      try {
        resolve(workspace, key, reference);
      } catch (error) {
        if (!isResolutionFailure(error))
          throw createError(
            "scope.internal",
            "Export validation failed",
            { file: scope.manifestFile, field: "exports", reference },
            error
          );
        throw createError(
          "scope.export_unresolved",
          `Export ${JSON.stringify(reference)} cannot be resolved`,
          { file: scope.manifestFile, field: "exports", reference },
          error
        );
      }
    }
  }
  const identities = new Map<string, RelationDeclaration["source"]>();
  const relations: Relation[] = [];
  for (const key of keys) {
    const scope = workspace.scopes.get(key) as Scope;
    for (const declaration of scope.relationDecls) {
      let from: EntityKey;
      try {
        from = resolve(workspace, key, declaration.from);
      } catch (error) {
        if (!isResolutionFailure(error))
          throw createError(
            "scope.internal",
            "Relation start validation failed",
            {
              file: declaration.source.file,
              line: String(declaration.source.line ?? 0),
              field: "relations.from",
              reference: declaration.from
            },
            error
          );
        throw createError(
          "scope.relation_start_unresolved",
          `Relation start reference ${JSON.stringify(declaration.from)} cannot be resolved`,
          {
            file: declaration.source.file,
            line: String(declaration.source.line ?? 0),
            field: "relations.from",
            reference: declaration.from
          },
          error
        );
      }
      let to: EntityKey;
      try {
        to = resolve(workspace, key, declaration.to);
      } catch (error) {
        if (!isResolutionFailure(error))
          throw createError(
            "scope.internal",
            "Relation end validation failed",
            {
              file: declaration.source.file,
              line: String(declaration.source.line ?? 0),
              field: "relations.to",
              reference: declaration.to
            },
            error
          );
        throw createError(
          "scope.relation_end_unresolved",
          `Relation end reference ${JSON.stringify(declaration.to)} cannot be resolved`,
          {
            file: declaration.source.file,
            line: String(declaration.source.line ?? 0),
            field: "relations.to",
            reference: declaration.to
          },
          error
        );
      }
      const identity = `${from.scope}\0${from.id}\0${declaration.type}\0${to.scope}\0${to.id}`;
      const previous = identities.get(identity);
      if (previous) {
        throw createError("scope.relation_duplicate", "Relation duplicates another relation declaration", {
          file: declaration.source.file,
          line: String(declaration.source.line ?? 0),
          field: "relations",
          reference: relationText(declaration)
        });
      }
      identities.set(identity, declaration.source);
      relations.push({
        from,
        type: declaration.type,
        to,
        fromRef: declaration.from,
        toRef: declaration.to,
        properties: declaration.properties,
        source: declaration.source
      });
    }
  }
  relations.sort(compareRelations);
  workspace.relations = relations;
}

function isResolutionFailure(error: unknown): boolean {
  return (
    error !== null &&
    typeof error === "object" &&
    "reason" in error &&
    (error.reason === "scope.object_not_found" || error.reason === "scope.reference_unresolved")
  );
}
function relationText(declaration: RelationDeclaration): string {
  return `${declaration.from} ${declaration.type} ${declaration.to}`;
}
export const Validate = validateWorkspace;
export const CompareRelations = compareRelations;
