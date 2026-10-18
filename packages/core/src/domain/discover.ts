import { readdir, readFile } from "node:fs/promises";
import type { Dirent } from "node:fs";
import path from "node:path";
import { createError, ERROR_REASON_CODES, sourceError, type ErrorReason } from "./errors.js";

export const MANIFEST_FILE_NAMES = ["locus.json", "locus.yaml", "locus.yml"] as const;
const DEFINITION_SUFFIXES = [".locus.json", ".locus.yaml", ".locus.yml"] as const;

interface IgnorePattern {
  readonly value: string;
  readonly directoryOnly: boolean;
  readonly basenameOnly: boolean;
  readonly expression: RegExp;
}

export interface ScopeDocumentPolicy {
  isIgnored(relative: string, directory: boolean): boolean;
  isDefinitionIncluded(relative: string): boolean;
}

export interface DiscoveredDocuments {
  readonly manifests: string[];
  readonly definitions: string[];
}

function globExpression(pattern: string): RegExp {
  let expression = "^";
  for (let index = 0; index < pattern.length; index += 1) {
    const character = pattern[index];
    if (character === undefined) break;
    if (character === "*") expression += "[^/]*";
    else if (character === "?") expression += "[^/]";
    else if (character === "[") {
      const close = pattern.indexOf("]", index + 1);
      if (close < 0) throw createError("scope.definition_invalid", "Invalid character class in ignore pattern");
      else {
        let content = pattern.slice(index + 1, close);
        if (content.startsWith("!")) content = `^${content.slice(1)}`;
        expression += `[${content}]`;
        index = close;
      }
    } else expression += /[\\^$+.(){}|]/u.test(character) ? `\\${character}` : character;
  }
  try {
    return new RegExp(`${expression}$`, "u");
  } catch (error) {
    if (!(error instanceof SyntaxError))
      throw createError("scope.internal", "Cannot compile ignore pattern", undefined, error);
    throw createError("scope.definition_invalid", "Invalid character class in ignore pattern", undefined, error);
  }
}

function matches(pattern: IgnorePattern, relative: string, directory: boolean): boolean {
  if (pattern.directoryOnly && !directory) return false;
  const candidate = pattern.basenameOnly ? path.posix.basename(relative) : relative;
  return pattern.expression.test(candidate);
}

async function readIgnorePatterns(directory: string): Promise<IgnorePattern[]> {
  const filename = path.join(directory, ".locusignore");
  let content: string;
  try {
    content = await readFile(filename, "utf8");
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return [];
    throw sourceError(error, "Cannot read .locusignore", { file: ".locusignore" });
  }
  const patterns: IgnorePattern[] = [];
  const lines = content.split(/\r?\n/u);
  for (let index = 0; index < lines.length; index += 1) {
    const original = lines[index] ?? "";
    const line = original.replace(/^\uFEFF/u, "").trim();
    if (line.length === 0 || line.startsWith("#")) continue;
    const details = { file: ".locusignore", line: String(index + 1), field: "pattern" };
    if (line.startsWith("!"))
      throw createError(
        "scope.definition_invalid",
        `.locusignore:${index + 1}: negated patterns are not supported`,
        details
      );
    if (line.includes("\\"))
      throw createError(
        "scope.definition_invalid",
        `.locusignore:${index + 1}: ignore patterns must use '/' separators`,
        details
      );
    const directoryOnly = line.endsWith("/");
    const anchored = line.startsWith("/");
    const value = line.replace(/^\//u, "").replace(/\/$/u, "");
    const segments = value.split("/");
    if (!value || segments.some(part => !part || part === "." || part === "..")) {
      throw createError(
        "scope.definition_invalid",
        `.locusignore:${index + 1}: invalid scope-relative ignore pattern`,
        details
      );
    }
    if (value.includes("**"))
      throw createError(
        "scope.definition_invalid",
        `.locusignore:${index + 1}: '**' patterns are not supported`,
        details
      );
    try {
      const expression = globExpression(value);
      patterns.push({ value, directoryOnly, basenameOnly: !anchored && !value.includes("/"), expression });
    } catch (error) {
      const reason =
        error &&
        typeof error === "object" &&
        "reason" in error &&
        typeof error.reason === "string" &&
        Object.hasOwn(ERROR_REASON_CODES, error.reason)
          ? (error.reason as ErrorReason)
          : "scope.internal";
      throw createError(reason, `.locusignore:${index + 1}: cannot compile ignore pattern`, details, error);
    }
  }
  return patterns;
}
export async function readScopeDocumentPolicy(directory: string): Promise<ScopeDocumentPolicy> {
  const patterns = await readIgnorePatterns(directory);
  const isIgnored = (relative: string, directoryEntry: boolean): boolean =>
    (directoryEntry && (path.posix.basename(relative) === ".git" || path.posix.basename(relative) === ".locus")) ||
    patterns.some(pattern => matches(pattern, relative, directoryEntry));
  return {
    isIgnored,
    isDefinitionIncluded(relative: string): boolean {
      const segments = relative.split("/");
      for (let index = 1; index < segments.length; index += 1) {
        if (isIgnored(segments.slice(0, index).join("/"), true)) return false;
      }
      return !isIgnored(relative, false);
    }
  };
}


function hasManifest(entries: readonly Dirent[]): boolean {
  return entries.some(entry => !entry.isDirectory() && (MANIFEST_FILE_NAMES as readonly string[]).includes(entry.name));
}

function isDefinitionFile(name: string): boolean {
  return DEFINITION_SUFFIXES.some(suffix => name.endsWith(suffix));
}

export async function discoverScopeDocuments(directory: string): Promise<DiscoveredDocuments> {
  let entries: Dirent[];
  try {
    entries = await readdir(directory, { withFileTypes: true });
  } catch (error) {
    throw sourceError(error, "Cannot read scope directory");
  }
  const manifests = entries
    .filter(entry => !entry.isDirectory() && (MANIFEST_FILE_NAMES as readonly string[]).includes(entry.name))
    .map(entry => entry.name);
  const policy = await readScopeDocumentPolicy(directory);
  const definitions: string[] = [];

  async function walk(current: string, relativeDirectory: string, currentEntries: readonly Dirent[]): Promise<void> {
    for (const entry of currentEntries) {
      const relative = relativeDirectory ? path.posix.join(relativeDirectory, entry.name) : entry.name;
      if (entry.isDirectory()) {
        if (policy.isIgnored(relative, true))
          continue;
        const child = path.join(current, entry.name);
        let childEntries: Dirent[];
        try {
          childEntries = await readdir(child, { withFileTypes: true });
        } catch (error) {
          throw sourceError(error, `Cannot read scope directory ${relative}`, { file: relative });
        }
        if (hasManifest(childEntries)) continue;
        await walk(child, relative, childEntries);
      } else if (
        !MANIFEST_FILE_NAMES.includes(entry.name as (typeof MANIFEST_FILE_NAMES)[number]) &&
        isDefinitionFile(entry.name) &&
        policy.isDefinitionIncluded(relative)
      ) {
        definitions.push(relative);
      }
    }
  }
  await walk(directory, "", entries);
  manifests.sort();
  definitions.sort();
  return { manifests, definitions };
}
export const DiscoverScopeDocuments = discoverScopeDocuments;
