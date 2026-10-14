import { readFile } from "node:fs/promises";
import path from "node:path";
import { LineCounter, parseAllDocuments, type Document, type YAMLMap, type YAMLSeq } from "yaml";
import { z } from "zod";
import { discoverScopeDocuments } from "./discover.js";
import { createError, sourceError, type ErrorReason } from "./errors.js";
import { Scope, type Entity, type Provenance, type RelationDeclaration, type Source } from "./types.js";

const manifestSchema = z
  .object({
    id: z.unknown().optional(),
    imports: z.record(z.string(), z.string()).nullable().optional(),
    exports: z.array(z.string()).nullable().optional()
  })
  .strict();
const definitionSchema = z
  .object({
    group: z.string().nullable().optional(),
    entities: z.array(z.unknown()).nullable().optional(),
    relations: z.array(z.unknown()).nullable().optional()
  })
  .strict();
const entitySchema = z.object({ id: z.string() }).catchall(z.unknown());
const relationSchema = z.object({ from: z.string(), type: z.string(), to: z.string() }).catchall(z.unknown());
type ParsedYaml = { readonly value: unknown; readonly document: Document; readonly lineCounter: LineCounter };
function parseSchema<T>(
  schema: z.ZodType<T>,
  parsed: ParsedYaml,
  file: string,
  reason: ErrorReason,
  value: unknown = parsed.value,
  field?: string,
  line?: number
): T {
  const result = schema.safeParse(value);
  if (result.success) return result.data;
  const issuePath = result.error.issues[0]?.path ?? [];
  const location = field ?? issuePath.join(".");
  const issueLine = line ?? lineAt(parsed, parsed.document.getIn(issuePath as (string | number)[], true));
  throw createError(
    reason,
    `${file}: line ${issueLine}: ${location || "document"} does not match the required structure`,
    { file, line: String(issueLine), ...(location ? { field: location } : {}) },
    result.error
  );
}
async function parseFile(filename: string, file: string, reason: ErrorReason): Promise<ParsedYaml> {
  let data: string;
  try {
    data = await readFile(filename, "utf8");
  } catch (error) {
    throw sourceError(error, `Cannot read ${file}`, { file });
  }
  const lineCounter = new LineCounter();
  let documents: Document[];
  try {
    documents = parseAllDocuments(data, { strict: true, lineCounter });
  } catch (error) {
    const parserReason = error instanceof Error && error.name === "YAMLParseError" ? reason : "scope.internal";
    throw createError(
      parserReason,
      `${file}: cannot parse YAML/JSON document`,
      { file, line: "1", field: "document" },
      error
    );
  }
  if (documents.length > 1)
    throw createError(reason, `${file}: multiple YAML/JSON documents are not supported`, {
      file,
      line: "1",
      field: "document"
    });
  const document = documents[0];
  if (!document) throw createError(reason, `${file}: document is empty`, { file, line: "1", field: "document" });
  const diagnostic = document.errors[0] ?? document.warnings[0];
  if (diagnostic) {
    const line = lineCounter.linePos(diagnostic.pos[0]).line;
    throw createError(
      reason,
      `${file}: line ${line}: invalid YAML/JSON document`,
      { file, line: String(line), field: "document" },
      diagnostic
    );
  }
  let value: unknown;
  try {
    value = document.toJS();
  } catch (error) {
    throw createError(
      error instanceof ReferenceError ? reason : "scope.internal",
      `${file}: cannot decode document`,
      { file, line: "1", field: "document" },
      error
    );
  }
  return { value, document, lineCounter };
}

function lineAt(parsed: ParsedYaml, node: unknown): number {
  const range = (node as { range?: readonly number[] } | undefined)?.range;
  if (!range || range.length === 0) return 0;
  return parsed.lineCounter.linePos(range[0] ?? 0).line;
}
function childLine(parsed: ParsedYaml, key: string, index: number): number {
  const contents = parsed.document.contents as YAMLMap | null;
  if (!contents || !("items" in contents)) return 0;
  const pair = contents.items.find(item => String((item.key as { toJSON?: () => unknown } | null)?.toJSON?.()) === key);
  const sequence = pair?.value as YAMLSeq | undefined;
  const item = sequence && "items" in sequence ? sequence.items[index] : undefined;
  return lineAt(parsed, item);
}
function objectProperties(value: Record<string, unknown>, excluded: readonly string[]): Record<string, unknown> {
  const properties: Record<string, unknown> = {};
  for (const [key, entry] of Object.entries(value)) if (!excluded.includes(key)) properties[key] = entry;
  return properties;
}

export async function decodeScope(source: Source): Promise<Scope> {
  const discovered = await discoverScopeDocuments(source.localPath);
  if (discovered.manifests.length === 0)
    throw createError(
      "scope.manifest_missing",
      "Scope manifest is missing; expected locus.yaml, locus.yml, or locus.json"
    );
  if (discovered.manifests.length > 1)
    throw createError("scope.manifest_ambiguous", `Multiple scope manifests found: ${discovered.manifests.join(", ")}`);
  const manifestFile = discovered.manifests[0] as string;
  const manifestPath = path.join(source.localPath, manifestFile);
  const parsedManifest = await parseFile(manifestPath, manifestFile, "scope.manifest_invalid");
  const rawManifest = parseSchema(manifestSchema, parsedManifest, manifestFile, "scope.manifest_invalid");
  const manifestFailure = (field: string, message: string): never => {
    const line = lineAt(parsedManifest, parsedManifest.document.getIn(field.split("."), true));
    throw createError("scope.manifest_invalid", `${manifestFile}: line ${line}: ${message}`, {
      file: manifestFile,
      line: String(line),
      field
    });
  };
  if (typeof rawManifest.id !== "string" || rawManifest.id.trim().length === 0)
    manifestFailure("id", "scope id is required");
  const imports = rawManifest.imports ?? {};
  const exportsList = rawManifest.exports ?? [];
  for (const [alias, importPath] of Object.entries(imports)) {
    if (alias.trim().length === 0) manifestFailure("imports", "import projection name must not be empty");
    if (alias.includes(":")) manifestFailure("imports", "import projection names must not contain ':'");
    if (importPath.trim().length === 0) manifestFailure("imports", "import source path must not be empty");
  }
  exportsList.forEach(ref => {
    if (ref.trim().length === 0) manifestFailure("exports", "export must be a non-empty entity reference");
  });
  const result = new Scope(source, { id: rawManifest.id as string, imports, exports: exportsList }, manifestFile);
  for (const relative of discovered.definitions) await decodeDefinition(result, path.join(source.localPath, relative));
  return result;
}
export async function decodeDefinition(scope: Scope, filename: string): Promise<void> {
  const relative = path.relative(scope.localPath, filename).split(path.sep).join("/");
  const parsed = await parseFile(filename, relative, "scope.definition_invalid");
  const raw = parseSchema(definitionSchema, parsed, relative, "scope.definition_invalid");
  const group = raw.group ?? "";
  if (group.includes(":"))
    throw createError("scope.definition_invalid", `${relative}: group must not contain ':'`, {
      file: relative,
      field: "group",
      line: String(lineAt(parsed, parsed.document.get("group", true)))
    });
  const entities = raw.entities ?? [];
  const relations = raw.relations ?? [];
  const declared = new Set<string>();
  for (let index = 0; index < entities.length; index += 1) {
    const rawEntity = entities[index];
    const entityLine = childLine(parsed, "entities", index);
    if (!rawEntity || typeof rawEntity !== "object" || Array.isArray(rawEntity)) {
      throw createError("scope.definition_invalid", `${relative}: line ${entityLine}: entity must be an object`, {
        file: relative,
        line: String(entityLine),
        field: "entities"
      });
    }
    const rawEntityRecord = rawEntity as Record<string, unknown>;
    if (typeof rawEntityRecord.id !== "string" || rawEntityRecord.id.trim().length === 0) {
      throw createError("scope.entity_id_invalid", `${relative}: line ${entityLine}: entity id is required`, {
        file: relative,
        line: String(entityLine),
        field: "entities.id"
      });
    }
    if (rawEntityRecord.id.includes(":")) {
      throw createError("scope.entity_id_invalid", `${relative}: line ${entityLine}: entity id must not contain ':'`, {
        file: relative,
        line: String(entityLine),
        field: "entities.id"
      });
    }
    const entity = parseSchema(
      entitySchema,
      parsed,
      relative,
      "scope.entity_id_invalid",
      rawEntity,
      "entities.id",
      entityLine
    );
    declared.add(entity.id);
    const id = group ? `${group}/${entity.id}` : entity.id;
    const previous = scope.entities.get(id);
    if (previous)
      throw createError(
        "scope.entity_duplicate",
        `${relative}: line ${entityLine}: entity conflicts with a declaration in ${previous.source.file} after group expansion`,
        { file: relative, line: String(entityLine), field: "entities.id", reference: id }
      );
    const source: Provenance = {
      scope: scope.key,
      file: relative,
      ...(group ? { group } : {}),
      line: entityLine,
      index: index + 1,
      path: filename
    };
    const value: Entity = { id, localId: entity.id, properties: objectProperties(entity, ["id"]), source };
    scope.entities.set(id, value);
  }
  for (let index = 0; index < relations.length; index += 1) {
    const rawRelation = relations[index];
    const relationLine = childLine(parsed, "relations", index);
    if (!rawRelation || typeof rawRelation !== "object" || Array.isArray(rawRelation)) {
      throw createError("scope.relation_invalid", `${relative}: line ${relationLine}: relation must be an object`, {
        file: relative,
        line: String(relationLine),
        field: "relations"
      });
    }
    const rawRelationRecord = rawRelation as Record<string, unknown>;
    if (
      typeof rawRelationRecord.from !== "string" ||
      typeof rawRelationRecord.type !== "string" ||
      typeof rawRelationRecord.to !== "string" ||
      !rawRelationRecord.from.trim() ||
      !rawRelationRecord.type.trim() ||
      !rawRelationRecord.to.trim()
    ) {
      throw createError(
        "scope.relation_invalid",
        `${relative}: line ${relationLine}: relation from, type, and to must be non-empty strings`,
        { file: relative, line: String(relationLine), field: "relations" }
      );
    }
    const relation = parseSchema(
      relationSchema,
      parsed,
      relative,
      "scope.relation_invalid",
      rawRelation,
      "relations",
      relationLine
    );
    let from = relation.from;
    let to = relation.to;
    if (!from.includes(":") && group && declared.has(from)) from = `${group}/${from}`;
    if (!to.includes(":") && group && declared.has(to)) to = `${group}/${to}`;
    const source: Provenance = {
      scope: scope.key,
      file: relative,
      ...(group ? { group } : {}),
      line: relationLine,
      index: index + 1,
      path: filename
    };
    const declaration: RelationDeclaration = {
      from,
      type: relation.type,
      to,
      properties: objectProperties(relation, ["from", "type", "to"]),
      source
    };
    scope.relationDecls.push(declaration);
  }
}

export async function decodeDefinitionFile(filename: string): Promise<unknown> {
  const parsed = await parseFile(filename, path.basename(filename), "scope.definition_invalid");
  return parsed.value;
}
export const DecodeScope = decodeScope;
