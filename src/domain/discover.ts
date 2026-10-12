import { readdir, readFile } from 'node:fs/promises';
import type { Dirent } from 'node:fs';
import path from 'node:path';

export const MANIFEST_FILE_NAMES = ['locus.json', 'locus.yaml', 'locus.yml'] as const;
const DEFINITION_SUFFIXES = ['.locus.json', '.locus.yaml', '.locus.yml'] as const;

interface IgnorePattern {
  readonly value: string;
  readonly directoryOnly: boolean;
  readonly basenameOnly: boolean;
  readonly expression: RegExp;
}

export interface DiscoveredDocuments {
  readonly manifests: string[];
  readonly definitions: string[];
}

function globExpression(pattern: string): RegExp {
  let expression = '^';
  for (let index = 0; index < pattern.length; index += 1) {
    const character = pattern[index];
    if (character === undefined) break;
    if (character === '*') expression += '[^/]*';
    else if (character === '?') expression += '[^/]';
    else if (character === '[') {
      const close = pattern.indexOf(']', index + 1);
      if (close < 0) throw new Error(`invalid character class in pattern ${pattern}`);
      else {
        let content = pattern.slice(index + 1, close);
        if (content.startsWith('!')) content = `^${content.slice(1)}`;
        expression += `[${content}]`;
        index = close;
      }
    } else expression += /[\\^$+.(){}|]/u.test(character) ? `\\${character}` : character;
  }
  return new RegExp(`${expression}$`, 'u');
}

function matches(pattern: IgnorePattern, relative: string, directory: boolean): boolean {
  if (pattern.directoryOnly && !directory) return false;
  const candidate = pattern.basenameOnly ? path.posix.basename(relative) : relative;
  return pattern.expression.test(candidate);
}

async function readIgnorePatterns(directory: string): Promise<IgnorePattern[]> {
  const filename = path.join(directory, '.locusignore');
  let content: string;
  try {
    content = await readFile(filename, 'utf8');
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') return [];
    throw new Error(`read ${filename}: ${(error as Error).message}`, { cause: error });
  }
  const patterns: IgnorePattern[] = [];
  const lines = content.split(/\r?\n/u);
  for (let index = 0; index < lines.length; index += 1) {
    const original = lines[index] ?? '';
    const line = original.replace(/^\uFEFF/u, '').trim();
    if (line.length === 0 || line.startsWith('#')) continue;
    if (line.startsWith('!')) throw new Error(`${filename}:${index + 1}: negated patterns are not supported`);
    if (line.includes('\\')) throw new Error(`${filename}:${index + 1}: ignore patterns must use '/' separators`);
    const directoryOnly = line.endsWith('/');
    const anchored = line.startsWith('/');
    const value = line.replace(/^\//u, '').replace(/\/$/u, '');
    const segments = value.split('/');
    if (!value || segments.some((part) => !part || part === '.' || part === '..')) {
      throw new Error(`${filename}:${index + 1}: invalid scope-relative ignore pattern ${JSON.stringify(original)}`);
    }
    if (value.includes('**')) throw new Error(`${filename}:${index + 1}: '**' patterns are not supported`);
    try {
      const expression = globExpression(value);
      patterns.push({ value, directoryOnly, basenameOnly: !anchored && !value.includes('/'), expression });
    } catch (error) {
      throw new Error(`${filename}:${index + 1}: invalid ignore pattern ${JSON.stringify(original)}: syntax error in pattern`, { cause: error });
    }
  }
  return patterns;
}

function hasManifest(entries: readonly Dirent[]): boolean {
  return entries.some((entry) => !entry.isDirectory() && (MANIFEST_FILE_NAMES as readonly string[]).includes(entry.name));
}

function isDefinitionFile(name: string): boolean {
  return DEFINITION_SUFFIXES.some((suffix) => name.endsWith(suffix));
}

export async function discoverScopeDocuments(directory: string): Promise<DiscoveredDocuments> {
  const entries = await readdir(directory, { withFileTypes: true });
  const manifests = entries.filter((entry) => !entry.isDirectory() && (MANIFEST_FILE_NAMES as readonly string[]).includes(entry.name)).map((entry) => entry.name);
  const patterns = await readIgnorePatterns(directory);
  const definitions: string[] = [];

  async function walk(current: string, relativeDirectory: string, currentEntries: readonly Dirent[]): Promise<void> {
    for (const entry of currentEntries) {
      const relative = relativeDirectory ? path.posix.join(relativeDirectory, entry.name) : entry.name;
      if (entry.isDirectory()) {
        if (entry.name === '.git' || entry.name === '.locus' || patterns.some((pattern) => matches(pattern, relative, true))) continue;
        const child = path.join(current, entry.name);
        const childEntries = await readdir(child, { withFileTypes: true });
        if (hasManifest(childEntries)) continue;
        await walk(child, relative, childEntries);
      } else if (!MANIFEST_FILE_NAMES.includes(entry.name as (typeof MANIFEST_FILE_NAMES)[number]) && isDefinitionFile(entry.name) && !patterns.some((pattern) => matches(pattern, relative, false))) {
        definitions.push(relative);
      }
    }
  }
  await walk(directory, '', entries);
  manifests.sort();
  definitions.sort();
  return { manifests, definitions };
}
export const DiscoverScopeDocuments = discoverScopeDocuments;
