import { realpath } from "node:fs/promises";
import path from "node:path";
import { pathToFileURL } from "node:url";
import { decodeDefinition } from "./decode.js";
import { sourceError } from "./errors.js";
import { validateWorkspace } from "./validate.js";
import { Scope, Workspace, type Source } from "./types.js";

export async function loadDefinitionFile(filename: string): Promise<Workspace> {
  const absolute = path.resolve(filename);
  let canonical: string;
  try {
    canonical = await realpath(absolute);
  } catch (error) {
    throw sourceError(error, "Cannot resolve definition file", { file: path.basename(filename) });
  }
  const key = pathToFileURL(canonical).toString();
  const source: Source = { key, localPath: path.dirname(canonical) };
  const current = new Scope(
    source,
    { id: path.basename(canonical), imports: {}, exports: [] },
    path.basename(canonical)
  );
  await decodeDefinition(current, canonical);
  const workspace = new Workspace(key, new Map([[key, current]]));
  validateWorkspace(workspace);
  return workspace;
}
export const LoadDefinitionFile = loadDefinitionFile;
