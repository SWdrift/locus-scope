import { readFile, realpath } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { LineCounter, parseAllDocuments, type Document, type YAMLMap, type YAMLSeq } from 'yaml';
import { validateWorkspace } from './validate.js';
import { Scope, Workspace, type Source } from './types.js';

function declarationLine(document: Document, lineCounter: LineCounter, field: string, index: number): number {
  const contents = document.contents as YAMLMap | null;
  if (!contents || !('items' in contents)) return 0;
  const pair = contents.items.find(item => String((item.key as { toJSON?: () => unknown } | null)?.toJSON?.()) === field);
  const sequence = pair?.value as YAMLSeq | undefined;
  const item = sequence && 'items' in sequence ? sequence.items[index] as { range?: readonly number[] } | undefined : undefined;
  const offset = item?.range?.[0];
  return offset === undefined ? 0 : lineCounter.linePos(offset).line;
}

export async function loadDefinitionFile(filename: string): Promise<Workspace> {
  const absolute = path.resolve(filename);
  const canonical = await realpath(absolute);
  const key = pathToFileURL(canonical).toString();
  const source: Source = { key, localPath: path.dirname(canonical) };
  const current = new Scope(source, { id: path.basename(canonical), imports: {}, exports: [] }, path.basename(canonical));
  const content = await readFile(canonical, 'utf8');
  const lineCounter = new LineCounter();
  const documents = parseAllDocuments(content, { strict: true, lineCounter });
  if (documents.length > 1) throw new Error(`${canonical}: multiple YAML/JSON documents are not supported`);
  const document = documents[0];
  if (!document) throw new Error(`${canonical}: invalid YAML`);
  if (document.errors.length > 0) throw new Error(`${canonical}: ${document.errors[0]?.message ?? 'invalid YAML'}`);
  if (document.warnings.length > 0) throw new Error(`${canonical}: ${document.warnings[0]?.message ?? 'invalid YAML'}`);
  const rawValue = document.toJS();
  if (!rawValue || typeof rawValue !== 'object' || Array.isArray(rawValue)) throw new Error(`${canonical}: definition must be an object`);
  const raw = rawValue as Record<string, unknown>;
  const allowed = new Set(['group', 'entities', 'relations']);
  for (const name of Object.keys(raw)) if (!allowed.has(name)) throw new Error(`${canonical}: unknown field ${JSON.stringify(name)}`);
  if (raw.group !== undefined && raw.group !== null && typeof raw.group !== 'string') throw new Error(`${canonical}: group must be a string`);
  if (raw.entities !== undefined && raw.entities !== null && !Array.isArray(raw.entities)) throw new Error(`${canonical}: entities must be an array`);
  if (raw.relations !== undefined && raw.relations !== null && !Array.isArray(raw.relations)) throw new Error(`${canonical}: relations must be an array`);
  const group = typeof raw.group === 'string' ? raw.group : '';
  if (group.includes(':')) throw new Error(`${canonical}: group ${JSON.stringify(group)} must not contain ':'`);
  const entities = Array.isArray(raw.entities) ? raw.entities : [];
  const relations = Array.isArray(raw.relations) ? raw.relations : [];
  const declared = new Set<string>();
  for (let index = 0; index < entities.length; index += 1) {
    const entity = entities[index];
    const entityLine = declarationLine(document, lineCounter, 'entities', index);
    if (!entity || typeof entity !== 'object' || Array.isArray(entity)) {
      throw new Error(`${canonical}: line ${entityLine}: entity ${index + 1} must be an object`);
    }
    const object = entity as Record<string, unknown>;
    if (typeof object.id !== 'string' || !object.id.trim()) {
      throw new Error(`${canonical}: line ${entityLine}: entity ${index + 1}: id is required`);
    }
    if (object.id.includes(':')) {
      throw new Error(`${canonical}: line ${entityLine}: entity ${JSON.stringify(object.id)}: id must not contain ':'`);
    }
    declared.add(object.id);
    const id = group ? `${group}/${object.id}` : object.id;
    if (current.entities.has(id)) throw new Error(`${canonical}: entity ${JSON.stringify(id)} is duplicated`);
    const properties = Object.fromEntries(Object.entries(object).filter(([name]) => name !== 'id'));
    current.entities.set(id, { id, localId: object.id, properties, source: { scope: key, file: path.basename(canonical), ...(group ? { group } : {}), line: entityLine, index: index + 1, path: canonical } });
  }
  for (let index = 0; index < relations.length; index += 1) {
    const relation = relations[index];
    const relationLine = declarationLine(document, lineCounter, 'relations', index);
    if (!relation || typeof relation !== 'object' || Array.isArray(relation)) {
      throw new Error(`${canonical}: line ${relationLine}: relation must be an object`);
    }
    const object = relation as Record<string, unknown>;
    if (typeof object.from !== 'string' || typeof object.type !== 'string' || typeof object.to !== 'string' ||
      !object.from.trim() || !object.type.trim() || !object.to.trim()) {
      throw new Error(`${canonical}: line ${relationLine}: relation from, type, and to must be non-empty strings`);
    }
    const from = group && !object.from.includes(':') && declared.has(object.from) ? `${group}/${object.from}` : object.from;
    const to = group && !object.to.includes(':') && declared.has(object.to) ? `${group}/${object.to}` : object.to;
    current.relationDecls.push({ from, type: object.type, to, properties: Object.fromEntries(Object.entries(object).filter(([name]) => !['from', 'type', 'to'].includes(name))), source: { scope: key, file: path.basename(canonical), ...(group ? { group } : {}), line: relationLine, index: index + 1, path: canonical } });
  }
  const workspace = new Workspace(key, new Map([[key, current]]));
  validateWorkspace(workspace);
  return workspace;
}
export const LoadDefinitionFile = loadDefinitionFile;
