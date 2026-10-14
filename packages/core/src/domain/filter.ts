import { createError } from "./errors.js";

export const FilterEqual = "=";
export const FilterNotEqual = "!=";
export const FilterPrefix = "^=";
export const FilterContains = "*=";
export const FilterRegexp = "~=";
export type FilterOperator =
  typeof FilterEqual | typeof FilterNotEqual | typeof FilterPrefix | typeof FilterContains | typeof FilterRegexp;

export interface Predicate {
  readonly path: string[];
  readonly metadata: boolean;
  readonly operator: FilterOperator;
  readonly value: unknown;
  readonly regexp?: RegExp;
}

export function parseFilters(argumentsList: string[]): Predicate[] {
  const predicates: Predicate[] = [];
  for (const argument of argumentsList) {
    const found = findFilterOperator(argument);
    if (!found || found.index <= 0)
      throw createError("scope.filter_invalid", "Filter requires a field and an operator");
    let field = argument.slice(0, found.index);
    const valueText = argument.slice(found.index + found.operator.length);
    const metadata = field.startsWith("@");
    if (metadata) field = field.slice(1);
    const parts = field.split(".");
    if (parts.some(part => part.length === 0))
      throw createError("scope.filter_invalid", "Filter field path contains an empty segment");
    let value: unknown;
    try {
      value = JSON.parse(valueText);
    } catch {
      value = valueText;
    }
    if (found.operator === FilterPrefix || found.operator === FilterContains || found.operator === FilterRegexp) {
      if (typeof value !== "string")
        throw createError("scope.filter_invalid", "Filter operator requires a string value");
    }
    let regexp: RegExp | undefined;
    if (found.operator === FilterRegexp) {
      try {
        regexp = new RegExp(value as string);
      } catch (error) {
        throw createError(
          error instanceof SyntaxError ? "scope.filter_invalid" : "scope.internal",
          "Cannot compile filter regular expression",
          undefined,
          error
        );
      }
    }
    predicates.push({ path: parts, metadata, operator: found.operator, value, ...(regexp ? { regexp } : {}) });
  }
  return predicates;
}

function findFilterOperator(
  argument: string
): { readonly operator: FilterOperator; readonly index: number } | undefined {
  const operators: FilterOperator[] = [FilterNotEqual, FilterPrefix, FilterContains, FilterRegexp, FilterEqual];
  for (let index = 0; index < argument.length; index += 1) {
    for (const operator of operators) if (argument.startsWith(operator, index)) return { operator, index };
  }
  return undefined;
}
function nestedField(root: Record<string, unknown>, parts: readonly string[]): { found: boolean; value?: unknown } {
  let current: unknown = root;
  for (const part of parts) {
    if (!current || typeof current !== "object" || Array.isArray(current)) return { found: false };
    const object = current as Record<string, unknown>;
    if (!Object.hasOwn(object, part)) return { found: false };
    current = object[part];
  }
  return { found: true, value: current };
}
function canonical(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(entry => canonical(entry));
  if (value && typeof value === "object")
    return Object.fromEntries(
      Object.entries(value)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([key, entry]) => [key, canonical(entry)])
    );
  return value;
}
function jsonEqual(left: unknown, right: unknown): boolean {
  return JSON.stringify(canonical(left)) === JSON.stringify(canonical(right));
}
function matchValue(actual: unknown, predicate: Predicate): boolean {
  if (predicate.operator === FilterEqual) return jsonEqual(actual, predicate.value);
  if (predicate.operator === FilterNotEqual) return !jsonEqual(actual, predicate.value);
  if (typeof actual !== "string" || typeof predicate.value !== "string") return false;
  if (predicate.operator === FilterPrefix) return actual.startsWith(predicate.value);
  if (predicate.operator === FilterContains) return actual.includes(predicate.value);
  return predicate.regexp?.test(actual) ?? false;
}

export function match(
  object: Record<string, unknown>,
  metadata: Record<string, unknown>,
  predicates: readonly Predicate[]
): boolean {
  for (const predicate of predicates) {
    const result = nestedField(predicate.metadata ? metadata : object, predicate.path);
    if (!result.found || !matchValue(result.value, predicate)) return false;
  }
  return true;
}
export const ParseFilters = parseFilters;
export const Match = match;
