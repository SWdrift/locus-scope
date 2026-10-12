import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { LineCounter, parseAllDocuments, type Document, type YAMLMap, type YAMLSeq } from 'yaml';
import { z } from 'zod';
import { discoverScopeDocuments } from './discover.js';
import { Scope, type Entity, type Provenance, type RelationDeclaration, type Source } from './types.js';

const manifestSchema = z.object({ id: z.unknown().optional(), imports: z.record(z.string(), z.string()).nullable().optional(), exports: z.array(z.string()).nullable().optional() }).strict();
const definitionSchema = z.object({ group: z.string().nullable().optional(), entities: z.array(z.unknown()).nullable().optional(), relations: z.array(z.unknown()).nullable().optional() }).strict();
const entitySchema = z.object({ id: z.string() }).catchall(z.unknown());
const relationSchema = z.object({ from: z.string(), type: z.string(), to: z.string() }).catchall(z.unknown());
type ParsedYaml = { readonly value: unknown; readonly document: Document; readonly lineCounter: LineCounter };
function parseSchema<T>(schema: z.ZodType<T>, value: unknown, filename: string): T {
  try { return schema.parse(value); }
  catch (error) { throw new Error(`${filename}: ${(error as Error).message}`, { cause: error }); }
}
async function parseFile(filename: string): Promise<ParsedYaml> {
  let data: string;
  try { data = await readFile(filename, 'utf8'); }
  catch (error) { throw new Error(`read ${filename}: ${(error as Error).message}`, { cause: error }); }
  const lineCounter = new LineCounter();
  const documents = parseAllDocuments(data, { strict: true, lineCounter });
  if (documents.length > 1) throw new Error(`${filename}: multiple YAML/JSON documents are not supported`);
  const document = documents[0];
  if (!document) throw new Error(`${filename}: invalid YAML`);
  if (document.errors.length > 0) throw new Error(`${filename}: ${document.errors[0]?.message ?? 'invalid YAML'}`);
  if (document.warnings.length > 0) throw new Error(`${filename}: ${document.warnings[0]?.message ?? 'invalid YAML'}`);
  return { value: document.toJS(), document, lineCounter };
}

function lineAt(parsed: ParsedYaml, node: unknown): number {
  const range = (node as { range?: readonly number[] } | undefined)?.range;
  if (!range || range.length === 0) return 0;
  return parsed.lineCounter.linePos(range[0] ?? 0).line;
}
function childLine(parsed: ParsedYaml, key: string, index: number): number {
  const contents = parsed.document.contents as YAMLMap | null;
  if (!contents || !('items' in contents)) return 0;
  const pair = contents.items.find((item) => String((item.key as { toJSON?: () => unknown } | null)?.toJSON?.()) === key);
  const sequence = pair?.value as YAMLSeq | undefined;
  const item = sequence && 'items' in sequence ? sequence.items[index] : undefined;
  return lineAt(parsed, item);
}
function objectProperties(value: Record<string, unknown>, excluded: readonly string[]): Record<string, unknown> {
  const properties: Record<string, unknown> = {};
  for (const [key, entry] of Object.entries(value)) if (!excluded.includes(key)) properties[key] = entry;
  return properties;
}

export async function decodeScope(source: Source): Promise<Scope> {
  const discovered = await discoverScopeDocuments(source.localPath);
  if (discovered.manifests.length === 0) throw new Error(`scope ${JSON.stringify(source.localPath)}: manifest missing; expected exactly one of locus.yaml, locus.yml, or locus.json`);
  if (discovered.manifests.length > 1) throw new Error(`scope ${JSON.stringify(source.localPath)}: multiple manifests found: ${discovered.manifests.join(', ')}`);
  const manifestFile = discovered.manifests[0] as string;
  const manifestPath = path.join(source.localPath, manifestFile);
  const parsedManifest = await parseFile(manifestPath);
  const rawManifest = parseSchema(manifestSchema, parsedManifest.value, manifestPath);
  if (typeof rawManifest.id !== 'string' || rawManifest.id.trim().length === 0) throw new Error(`${manifestPath}: scope id is required`);
  const imports = rawManifest.imports ?? {};
  const exportsList = rawManifest.exports ?? [];
  for (const [alias, importPath] of Object.entries(imports)) {
    if (alias.trim().length === 0) throw new Error(`${manifestPath}: import projection name must not be empty`);
    if (alias.includes(':')) throw new Error(`${manifestPath}: import projection ${JSON.stringify(alias)} must not contain ':'`);
    if (importPath.trim().length === 0) throw new Error(`${manifestPath}: import ${JSON.stringify(alias)} has an empty source path`);
  }
  exportsList.forEach((ref, index) => { if (ref.trim().length === 0) throw new Error(`${manifestPath}: export ${index + 1} must be a non-empty entity reference`); });
  const result = new Scope(source, { id: rawManifest.id, imports, exports: exportsList }, manifestFile);
  for (const relative of discovered.definitions) await decodeDefinition(result, path.join(source.localPath, relative));
  return result;
}
async function decodeDefinition(scope: Scope, filename: string): Promise<void> {
  const parsed = await parseFile(filename);
  const raw = parseSchema(definitionSchema, parsed.value, filename);
  const group = raw.group ?? '';
  if (group.includes(':')) throw new Error(`${filename}: group ${JSON.stringify(group)} must not contain ':'`);
  const entities = raw.entities ?? [];
  const relations = raw.relations ?? [];
  const relative = path.relative(scope.localPath, filename).split(path.sep).join('/');
  const declared = new Set<string>();
  for (let index = 0; index < entities.length; index += 1) {
    const rawEntity = entities[index];
    const entityLine = childLine(parsed, 'entities', index);
    if (!rawEntity || typeof rawEntity !== 'object' || Array.isArray(rawEntity)) {
      throw new Error(`${filename}: line ${entityLine}: entity ${index + 1} must be an object`);
    }
    const rawEntityRecord = rawEntity as Record<string, unknown>;
    if (typeof rawEntityRecord.id !== 'string' || rawEntityRecord.id.trim().length === 0) {
      throw new Error(`${filename}: line ${entityLine}: entity ${index + 1}: id is required`);
    }
    if (rawEntityRecord.id.includes(':')) {
      throw new Error(`${filename}: line ${entityLine}: entity ${JSON.stringify(rawEntityRecord.id)}: id must not contain ':'`);
    }
    const entity = parseSchema(entitySchema, rawEntity, filename);
    declared.add(entity.id);
    const id = group ? `${group}/${entity.id}` : entity.id;
    const previous = scope.entities.get(id);
    if (previous) throw new Error(`${filename}: entity ${JSON.stringify(id)} conflicts with entity declared in ${previous.source.path} after group expansion`);
    const source: Provenance = { scope: scope.key, file: relative, ...(group ? { group } : {}), line: entityLine, index: index + 1, path: filename };
    const value: Entity = { id, localId: entity.id, properties: objectProperties(entity, ['id']), source };
    scope.entities.set(id, value);
  }
  for (let index = 0; index < relations.length; index += 1) {
    const rawRelation = relations[index];
    const relationLine = childLine(parsed, 'relations', index);
    if (!rawRelation || typeof rawRelation !== 'object' || Array.isArray(rawRelation)) {
      throw new Error(`${filename}: line ${relationLine}: relation must be an object`);
    }
    const rawRelationRecord = rawRelation as Record<string, unknown>;
    if (typeof rawRelationRecord.from !== 'string' || typeof rawRelationRecord.type !== 'string' || typeof rawRelationRecord.to !== 'string' ||
      !rawRelationRecord.from.trim() || !rawRelationRecord.type.trim() || !rawRelationRecord.to.trim()) {
      throw new Error(`${filename}: line ${relationLine}: relation from, type, and to must be non-empty strings`);
    }
    const relation = parseSchema(relationSchema, rawRelation, filename);
    let from = relation.from;
    let to = relation.to;
    if (!from.includes(':') && group && declared.has(from)) from = `${group}/${from}`;
    if (!to.includes(':') && group && declared.has(to)) to = `${group}/${to}`;
    const source: Provenance = { scope: scope.key, file: relative, ...(group ? { group } : {}), line: relationLine, index: index + 1, path: filename };
    const declaration: RelationDeclaration = { from, type: relation.type, to, properties: objectProperties(relation, ['from', 'type', 'to']), source };
    scope.relationDecls.push(declaration);
  }
}

export async function decodeDefinitionFile(filename: string): Promise<unknown> {
  const parsed = await parseFile(filename);
  return parsed.value;
}
export const DecodeScope = decodeScope;
