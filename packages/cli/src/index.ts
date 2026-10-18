import { stdin as processStdin, stdout as processStdout, stderr as processStderr } from "node:process";
import { resolve } from "node:path";
import "@sundw/locus-scope-core";
import type { AdjacentRequest, ApplicationService, MutationRequest } from "@sundw/locus-scope-core";
import type { EntityKey, Workspace } from "@sundw/locus-scope-core";
import { ApplicationError, createError, normalizeError } from "@sundw/locus-scope-core";
import { parseFilters } from "@sundw/locus-scope-core";
import {
  environmentSchema,
  parseRequest,
  protocolVersion,
  type CliRequest,
  type CliResponse,
  type Environment
} from "./protocol.js";

export {
  decodeRequest,
  encodeRequest,
  encodeResponse,
  environmentSchema,
  packageDescriptorSchema,
  parseRequest,
  parseResponse,
  protocolVersion,
  requestSchema,
  responseSchema
} from "./protocol.js";
export type { CliRequest, CliResponse } from "./protocol.js";
export * from "@sundw/locus-scope-core";

export type CliIO = {
  stdin?: AsyncIterable<Uint8Array | string>;
  stdout?: { write(value: string): unknown };
  stderr?: { write(value: string): unknown };
};

export type QueryKind = "scope" | "group" | "entity" | "relation";

export type ApplicationErrorLike = Error & {
  code?: string;
  reason?: string;
  message: string;
};

export type { ApplicationService };

export type CliContext = CliIO & {
  workingDirectory?: string;
  environment?: Environment;
  discoverRoot?: (workingDirectory: string, explicitRoot?: string) => string | Promise<string>;
  buildEnvironment?: (scopeRoot: string) => Environment | Promise<Environment>;
  loadApplication?: (environment: Environment, request: CliRequest) => ApplicationService | Promise<ApplicationService>;
  application?:
    | ApplicationService
    | ((environment: Environment, request: CliRequest) => ApplicationService | Promise<ApplicationService>);
  readStdin?: (stdin: CliIO["stdin"]) => string | Promise<string>;
  version?: string;
  reloadApplication?: (
    environment: Environment,
    request: CliRequest
  ) => ApplicationService | Promise<ApplicationService>;
  loadPath?: (path: string) => Workspace | Promise<Workspace>;
  reloadWorkspace?: () => Workspace | Promise<Workspace>;
};
type ParsedArguments = {
  command: string;
  args: string[];
  json: boolean;
  source: boolean;
  scopeDirectory?: string;
};

const rootlessCommands = new Set(["", "help", "--help", "-h", "version", "--version"]);
const commandNames = new Set([
  "scope",
  "group",
  "entity",
  "relation",
  "graph",
  "path",
  "impact",
  "adjacent",
  "diff",
  "validate",
  "version",
  "help",
  "--version",
  "--help",
  "-h"
]);

function parseArguments(arguments_: string[]): ParsedArguments {
  const remaining: string[] = [];
  let json = false;
  let source = false;
  let scopeDirectory: string | undefined;
  let rootless: "help" | "version" | undefined;
  for (let index = 0; index < arguments_.length; index += 1) {
    const argument = arguments_[index];
    if (argument === undefined) continue;
    if (argument === "--json") {
      json = true;
      continue;
    }
    if (argument === "--source") {
      source = true;
      continue;
    }
    if (argument === "--help" || argument === "-h") {
      rootless = "help";
      continue;
    }
    if (argument === "--version") {
      rootless = "version";
      continue;
    }
    if (argument === "--scope") {
      const value = arguments_[index + 1];
      if (value === undefined || value.startsWith("-"))
        throw createError("scope.selector_invalid", "--scope requires a directory");
      scopeDirectory = value;
      index += 1;
      continue;
    }
    if (argument.startsWith("--scope=")) {
      scopeDirectory = argument.slice("--scope=".length);
      if (!scopeDirectory) throw createError("scope.selector_invalid", "--scope requires a directory");
      continue;
    }
    remaining.push(argument);
  }
  const command = rootless ?? remaining[0] ?? "";
  if (!rootless && command.startsWith("-")) throw createError("scope.selector_invalid", `unknown option ${command}`);
  if (!rootless && command && !commandNames.has(command))
    throw createError("scope.selector_invalid", `unknown command ${command}`);
  const args = rootless === "help" && remaining[0] === "help" ? remaining.slice(1) : rootless ? [] : remaining.slice(1);
  return { command, args, json, source, ...(scopeDirectory === undefined ? {} : { scopeDirectory }) };
}

function outputValue(value: unknown, json: boolean): string {
  if (!json && value && typeof value === "object" && "name" in value && "version" in value) {
    const version = value as { name: unknown; version: unknown };
    return `${String(version.name)} ${String(version.version)}\n`;
  }
  if (!json && typeof value === "string") return `${value}\n`;
  const encoded = JSON.stringify(value, null, json ? 0 : 2);
  return `${encoded === undefined ? "null" : encoded}\n`;
}

function sanitizePublicText(value: string): string {
  // Never echo a token, URI, absolute path, or multi-line input in public diagnostics.
  return (
    value
      .replace(/(?:[A-Za-z][A-Za-z0-9+.-]*:\/\/|[A-Za-z]:[\\/]|\\\\)[^\r\n]*/gu, "<redacted>")
      .replace(/(^|[\s"'(=])\/[^\r\n]*/gu, "$1<redacted>")
      .replace(/(?:_authToken|token|password|secret)\s*[=:]\s*[^\r\n]*/giu, "<redacted>")
      .split(/\r?\n/u)[0] ?? ""
  );
}

function errorResponse(arguments_: string[], error: unknown): CliResponse {
  const application = normalizeError(error, "scope.cli");
  const message = application.code === "internal" ? "scope operation failed" : sanitizePublicText(application.message);
  const details: Record<string, string> = {};
  if (application.code !== "internal") {
    for (const [key, value] of Object.entries(application.details ?? {})) {
      if (
        [
          "file",
          "line",
          "column",
          "field",
          "scope",
          "reference",
          "entity",
          "relation",
          "alias",
          "dependency",
          "operation"
        ].includes(key)
      ) {
        details[key] = sanitizePublicText(value);
      }
    }
  }
  const errorBody = {
    error: message,
    code: application.code,
    reason: application.reason,
    ...(Object.keys(details).length ? { details } : {})
  };
  const location = details.file ? ` (${details.file}${details.line ? `:${details.line}` : ""})` : "";
  const body = arguments_.includes("--json")
    ? `${JSON.stringify(errorBody)}\n`
    : `locus-scope: ${message}${location}\n`;
  return {
    version: protocolVersion,
    exitCode: application.code === "invalid_argument" ? 2 : 1,
    stdout: "",
    stderr: body
  };
}

async function readInput(context: CliContext, request: CliRequest): Promise<string> {
  if (request.stdin !== undefined) return request.stdin;
  if (context.readStdin) return context.readStdin(context.stdin ?? processStdin);
  const input = context.stdin ?? processStdin;
  const chunks: string[] = [];
  for await (const chunk of input) chunks.push(typeof chunk === "string" ? chunk : Buffer.from(chunk).toString("utf8"));
  return chunks.join("");
}

async function resolveEnvironment(
  parsed: ParsedArguments,
  request: CliRequest,
  context: CliContext
): Promise<Environment> {
  if (context.environment) return environmentSchema.parse(context.environment);
  if (
    parsed.command === "" ||
    parsed.command === "help" ||
    parsed.command === "version" ||
    parsed.command === "--help" ||
    parsed.command === "--version"
  ) {
    return { rootScope: "", rootDependencies: {}, packages: {} };
  }
  if (context.application && !context.environment) return { rootScope: "", rootDependencies: {}, packages: {} };
  const cwd = request.workingDirectory;
  if (!context.discoverRoot || !context.buildEnvironment)
    throw createError("scope.internal", "Scope environment loader is required");
  const root = await context.discoverRoot(cwd, parsed.scopeDirectory);
  return environmentSchema.parse(await context.buildEnvironment(root));
}

async function resolveApplication(request: CliRequest, context: CliContext): Promise<ApplicationService> {
  if (context.application) {
    const application =
      typeof context.application === "function"
        ? await context.application(request.environment, request)
        : context.application;
    return application;
  }
  if (context.loadApplication) return context.loadApplication(request.environment, request);
  throw createError("scope.internal", "Scope application loader is required");
}

function positionalAndFlags(args: string[], command: string): { positionals: string[]; flags: Map<string, string[]> } {
  const positionals: string[] = [];
  const flags = new Map<string, string[]>();
  const allowed =
    command === "graph"
      ? new Set(["--depth", "--via"])
      : command === "adjacent"
        ? new Set(["--direction", "--via"])
        : command === "path" || command === "impact"
          ? new Set(["--via"])
          : command === "entity" || command === "relation"
            ? new Set(["--file"])
            : new Set<string>();
  for (let index = 0; index < args.length; index += 1) {
    const value = args[index];
    if (value === undefined) continue;
    if (value.startsWith("--")) {
      const equal = value.indexOf("=");
      const name = equal < 0 ? value : value.slice(0, equal);
      const inline = equal < 0 ? undefined : value.slice(equal + 1);
      if (!allowed.has(name)) throw createError("scope.selector_invalid", `unknown option ${value}`);
      const next = inline ?? args[index + 1];
      if (next === undefined || (inline === undefined && next.startsWith("--")))
        throw createError("scope.selector_invalid", `${name} requires a value`);
      if (inline === undefined) index += 1;
      const values = flags.get(name) ?? [];
      values.push(next);
      flags.set(name, values);
      continue;
    }
    positionals.push(value);
  }
  if (command === "validate" && positionals.length)
    throw createError("scope.selector_invalid", "validate takes no arguments");
  return { positionals, flags };
}

async function callQuery(
  application: ApplicationService,
  kind: QueryKind,
  args: string[],
  source: boolean
): Promise<unknown> {
  if (kind !== "relation" && args.length === 1) {
    const value = args[0] as string;
    if (!/[=!^*~]/.test(value)) {
      if (kind === "scope") return application.getScope(value, source);
      if (kind === "group") return application.getGroup(value, source);
      return application.getEntity(value, source);
    }
    try {
      if (kind === "scope") return application.getScope(value, source);
      if (kind === "group") return application.getGroup(value, source);
      return application.getEntity(value, source);
    } catch (error) {
      if (!(error instanceof ApplicationError) || error.reason !== "scope.object_not_found") throw error;
    }
  }
  let predicates;
  try {
    predicates = parseFilters(args);
  } catch (error) {
    throw createError("scope.filter_invalid", error instanceof Error ? error.message : String(error));
  }
  if (kind === "scope") return application.queryScopes(predicates, source);
  if (kind === "group") return application.queryGroups(predicates, source);
  if (kind === "entity") return application.queryEntities(predicates, source);
  return application.queryRelations(predicates, source);
}
function mergeMutationObject(target: Record<string, unknown>, input: Record<string, unknown>): void {
  for (const [key, value] of Object.entries(input)) {
    if (Object.hasOwn(target, key) && JSON.stringify(target[key]) !== JSON.stringify(value)) {
      throw createError("scope.mutation_input_invalid", `positional ${key} does not match JSON value`);
    }
  }
  Object.defineProperties(target, Object.getOwnPropertyDescriptors(input));
}
function parseMutationPayload(
  kind: "entity" | "relation",
  action: string,
  arguments_: string[],
  file: string,
  stdin: string
): Record<string, unknown> {
  const values = [...arguments_];
  const result: Record<string, unknown> = { kind, action, object: {}, fields: {}, file };
  if (action === "unset") {
    const minimum = kind === "entity" ? 2 : 4;
    if (values.length < minimum)
      throw createError("scope.mutation_input_invalid", `${kind} unset requires object identity and fields`);
    if (kind === "entity") {
      result.ref = values.shift();
    } else {
      result.from = values.shift();
      result.type = values.shift();
      result.to = values.shift();
    }
    result.unset = values;
    return result;
  }
  if (action === "remove") {
    const expected = kind === "entity" ? 1 : 3;
    if (values.length !== expected)
      throw createError("scope.mutation_input_invalid", `${kind} remove requires object identity`);
    if (kind === "entity") result.ref = values[0];
    else {
      result.from = values[0];
      result.type = values[1];
      result.to = values[2];
    }
    return result;
  }
  const positionals: string[] = [];
  const payloads: string[] = [];
  for (const value of values) {
    if (value === "-" || value.trimStart().startsWith("{") || /[=!]/.test(value)) payloads.push(value);
    else positionals.push(value);
  }
  if (kind === "entity") {
    if (positionals.length > 1)
      throw createError("scope.mutation_input_invalid", "entity mutation accepts at most one id/ref");
    if (positionals.length === 1) {
      if (action === "add") (result.object as Record<string, unknown>).id = positionals[0];
      else result.ref = positionals[0];
    }
  } else {
    if (positionals.length !== 0 && positionals.length !== 3)
      throw createError(
        "scope.mutation_input_invalid",
        "relation mutation requires from type to or a complete JSON object"
      );
    if (positionals.length === 3) {
      result.from = positionals[0];
      result.type = positionals[1];
      result.to = positionals[2];
      Object.assign(result.object as Record<string, unknown>, {
        from: positionals[0],
        type: positionals[1],
        to: positionals[2]
      });
    }
  }
  for (const payload of payloads) {
    if (payload === "-") {
      let decoded: unknown;
      try {
        decoded = JSON.parse(stdin);
      } catch {
        throw createError("scope.mutation_input_invalid", "decode stdin JSON failed");
      }
      if (!decoded || typeof decoded !== "object" || Array.isArray(decoded))
        throw createError("scope.mutation_input_invalid", "mutation JSON must be an object");
      mergeMutationObject(result.object as Record<string, unknown>, decoded as Record<string, unknown>);
      continue;
    }
    if (payload.trimStart().startsWith("{")) {
      let decoded: unknown;
      try {
        decoded = JSON.parse(payload);
      } catch {
        throw createError("scope.mutation_input_invalid", "decode JSON object failed");
      }
      if (!decoded || typeof decoded !== "object" || Array.isArray(decoded))
        throw createError("scope.mutation_input_invalid", "mutation JSON must be an object");
      mergeMutationObject(result.object as Record<string, unknown>, decoded as Record<string, unknown>);
      continue;
    }
    const separator = payload.indexOf("=");
    if (separator <= 0)
      throw createError("scope.mutation_input_invalid", `invalid field assignment ${JSON.stringify(payload)}`);
    const field = payload.slice(0, separator);
    const encoded = payload.slice(separator + 1);
    let value: unknown = encoded;
    try {
      value = JSON.parse(encoded);
    } catch {
      // Assignment values that are not JSON remain strings by protocol design.
    }
    Object.defineProperty(result.fields, field, { value, enumerable: true, writable: true, configurable: true });
  }
  const object = result.object as Record<string, unknown>;
  if (kind === "entity") {
    const id = typeof object.id === "string" ? object.id : undefined;
    if (action === "add" && !id) throw createError("scope.mutation_input_invalid", "entity add requires id");
    if (action === "set") {
      if (!result.ref && id) result.ref = id;
      if (!result.ref) throw createError("scope.mutation_input_invalid", "entity set requires ref or JSON id");
      if (id && id !== result.ref)
        throw createError("scope.mutation_input_invalid", "positional ref and JSON id must match");
    }
  } else {
    for (const field of ["from", "type", "to"]) {
      if (typeof object[field] !== "string" && typeof result[field] !== "string")
        throw createError("scope.mutation_input_invalid", `relation ${field} is required`);
    }
    if (!result.from) {
      result.from = object.from;
      result.type = object.type;
      result.to = object.to;
    }
  }
  return result;
}

async function resolveEntityKey(application: ApplicationService, reference: string): Promise<EntityKey> {
  return application.getEntity(reference, false).key;
}

async function resolveSeeds(application: ApplicationService, input: string | undefined): Promise<EntityKey[]> {
  if (input === undefined) return [];
  if (input === "") throw createError("scope.graph_seed_invalid", "stdin Entity envelope is empty");
  let decoded: unknown;
  try {
    decoded = JSON.parse(input);
  } catch {
    return [await resolveEntityKey(application, input)];
  }
  const values = Array.isArray(decoded) ? decoded : [decoded];
  const seeds: EntityKey[] = [];
  for (const value of values) {
    if (!value || typeof value !== "object" || !("key" in value))
      throw createError("scope.graph_seed_invalid", "stdin Entity envelope requires key");
    const key = value.key;
    if (
      !key ||
      typeof key !== "object" ||
      !("scope" in key) ||
      !("id" in key) ||
      typeof key.scope !== "string" ||
      typeof key.id !== "string" ||
      !key.scope ||
      !key.id
    ) {
      throw createError("scope.graph_seed_invalid", "stdin Entity envelope requires key.scope and key.id");
    }
    seeds.push(key as EntityKey);
  }
  return seeds;
}
function callRootless(parsed: ParsedArguments, context: CliContext): unknown {
  if (parsed.command === "version") return { name: "locus-scope", version: context.version ?? "unknown" };
  return "Usage: locus-scope <scope|group|entity|relation|graph|path|impact|adjacent|validate|version>";
}

async function callCommand(
  application: ApplicationService,
  parsed: ParsedArguments,
  request: CliRequest,
  context: CliContext
): Promise<unknown> {
  const { positionals, flags } = positionalAndFlags(parsed.args, parsed.command);
  if (
    parsed.command === "scope" ||
    parsed.command === "group" ||
    parsed.command === "entity" ||
    parsed.command === "relation"
  ) {
    if (
      (parsed.command === "entity" || parsed.command === "relation") &&
      positionals[0] &&
      ["add", "set", "unset", "remove"].includes(positionals[0])
    ) {
      const action = positionals.shift() as "add" | "set" | "unset" | "remove";
      if (action !== "add" && flags.has("--file"))
        throw createError("scope.mutation_input_invalid", "--file is only valid with add");
      if (!context.reloadWorkspace) throw createError("scope.internal", "Scope workspace reloader is required");
      const stdin = positionals.includes("-") ? await readInput(context, request) : "";
      const fileValues = flags.get("--file");
      const file = fileValues?.[fileValues.length - 1] ?? "";
      const mutation = await application.mutate(
        parseMutationPayload(parsed.command, action, positionals, file, stdin) as unknown as MutationRequest,
        context.reloadWorkspace
      );
      return mutation.result;
    }
    if (flags.size) throw createError("scope.selector_invalid", `unknown option ${[...flags.keys()][0] as string}`);
    return callQuery(application, parsed.command, positionals, parsed.source);
  }
  if (parsed.command === "validate") {
    if (flags.size) throw createError("scope.selector_invalid", "validate takes no options");
    if (positionals.length) throw createError("scope.selector_invalid", "validate takes no arguments");
    return application.validate();
  }
  if (parsed.command === "graph" || parsed.command === "impact") {
    if (parsed.command === "impact" && positionals.length !== 1)
      throw createError("scope.selector_invalid", "impact requires one entity reference");
    if (parsed.command === "graph" && positionals.length > 1)
      throw createError("scope.selector_invalid", "graph accepts at most one entity reference");
    const depthValues = flags.get("--depth");
    const depthValue = depthValues?.[depthValues.length - 1];
    const depth = depthValue === undefined ? undefined : Number(depthValue);
    if (depthValue !== undefined && (!Number.isInteger(depth) || (depth as number) < 0))
      throw createError("scope.selector_invalid", "graph depth must be a non-negative integer");
    const seed = positionals[0];
    let seeds: EntityKey[];
    if (parsed.command === "graph" && seed === undefined) {
      const root = application.workspace.root;
      seeds = application
        .queryEntities([], false)
        .filter(item => item.key.scope === root)
        .map(item => item.key);
    } else {
      const input = seed === "-" ? await readInput(context, request) : seed;
      seeds = await resolveSeeds(application, input);
    }
    let via;
    try {
      via = parseFilters(flags.get("--via") ?? []);
    } catch (error) {
      throw createError("scope.filter_invalid", error instanceof Error ? error.message : String(error));
    }
    if (parsed.command === "graph") return application.graph(seeds, depth, via, parsed.source);
    return application.impact(seeds, via, parsed.source);
  }
  if (parsed.command === "adjacent") {
    if (positionals.length !== 1) throw createError("scope.selector_invalid", "adjacent requires one entity reference");
    const directionValue = flags.get("--direction")?.at(-1) ?? "both";
    if (directionValue !== "incoming" && directionValue !== "outgoing" && directionValue !== "both")
      throw createError("scope.selector_invalid", "adjacent direction must be incoming, outgoing, or both");
    const input = positionals[0] === "-" ? await readInput(context, request) : positionals[0];
    const entities = await resolveSeeds(application, input);
    let via;
    try {
      via = parseFilters(flags.get("--via") ?? []);
    } catch (error) {
      throw createError("scope.filter_invalid", error instanceof Error ? error.message : String(error));
    }
    const requestValue: AdjacentRequest = { entities, direction: directionValue, via };
    return application.adjacent(requestValue, parsed.source);
  }
  if (parsed.command === "path") {
    if (positionals.length !== 2) throw createError("scope.selector_invalid", "path requires from and to references");
    const from = await resolveEntityKey(application, positionals[0] as string);
    const to = await resolveEntityKey(application, positionals[1] as string);
    let via;
    try {
      via = parseFilters(flags.get("--via") ?? []);
    } catch (error) {
      throw createError("scope.filter_invalid", error instanceof Error ? error.message : String(error));
    }
    return application.path(from, to, via, parsed.source);
  }
  if (parsed.command === "diff") {
    if (positionals.length !== 2) throw createError("scope.diff_input_invalid", "diff requires two selectors");
    if (!context.loadPath) throw createError("scope.internal", "Scope path loader is required");
    return application.diff(positionals[0] as string, positionals[1] as string, context.loadPath);
  }
  throw createError("scope.selector_invalid", `unknown command ${parsed.command}`);
}
export async function execute(requestValue: CliRequest, context: CliContext = {}): Promise<CliResponse> {
  try {
    const request = parseRequest(requestValue);
    const parsed = parseArguments(request.arguments);
    const rootless = rootlessCommands.has(parsed.command);
    const result = rootless
      ? callRootless(parsed, context)
      : await callCommand(await resolveApplication(request, context), parsed, request, context);
    return { version: protocolVersion, exitCode: 0, stdout: outputValue(result, parsed.json), stderr: "" };
  } catch (error) {
    const arguments_ =
      typeof requestValue === "object" &&
      requestValue !== null &&
      "arguments" in requestValue &&
      Array.isArray(requestValue.arguments)
        ? requestValue.arguments.filter((value): value is string => typeof value === "string")
        : [];
    return errorResponse(arguments_, error);
  }
}

export function run(request: CliRequest, context?: CliContext): Promise<CliResponse>;
export function run(arguments_: string[], context?: CliContext): Promise<CliResponse>;
export async function run(value: CliRequest | string[], context: CliContext = {}): Promise<CliResponse> {
  if (!Array.isArray(value)) return execute(value, context);
  const arguments_ = value;
  const workingDirectory = resolve(context.workingDirectory ?? process.cwd());
  try {
    const preliminary = parseArguments(arguments_);
    const environment = await resolveEnvironment(
      preliminary,
      {
        version: protocolVersion,
        workingDirectory,
        arguments: arguments_,
        environment: { rootScope: "", rootDependencies: {}, packages: {} }
      },
      context
    );
    const stdin = preliminary.args.includes("-")
      ? await readInput(context, { version: protocolVersion, workingDirectory, arguments: arguments_, environment })
      : undefined;
    const request: CliRequest = {
      version: protocolVersion,
      workingDirectory,
      arguments: arguments_,
      environment,
      ...(stdin === undefined ? {} : { stdin })
    };
    return await execute(request, context);
  } catch (error) {
    return errorResponse(arguments_, error);
  }
}

export async function main(arguments_: string[], context: CliContext = {}): Promise<number> {
  const response = await run(arguments_, context);
  const stdout = context.stdout ?? processStdout;
  const stderr = context.stderr ?? processStderr;
  if (response.stdout) stdout.write(response.stdout);
  if (response.stderr) stderr.write(response.stderr);
  return response.exitCode;
}

export function createLibraryBridge(context: CliContext = {}) {
  return {
    run: (arguments_: string[]) => run(arguments_, context),
    execute: (request: CliRequest) => execute(request, context),
    main: (arguments_: string[]) => main(arguments_, context)
  };
}
