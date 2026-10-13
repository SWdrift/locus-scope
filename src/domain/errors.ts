export type ErrorCode =
  "invalid_argument" | "invalid_data" | "not_found" | "conflict" | "permission_denied" | "internal";

export const ERROR_REASON_CODES = {
  "scope.selector_invalid": "invalid_argument",
  "scope.filter_invalid": "invalid_argument",
  "scope.reference_unresolved": "invalid_argument",
  "scope.graph_seed_invalid": "invalid_argument",
  "scope.field_invalid": "invalid_argument",
  "scope.diff_input_invalid": "invalid_argument",
  "scope.mutation_input_invalid": "invalid_argument",
  "scope.source_not_found": "not_found",
  "scope.object_not_found": "not_found",
  "scope.path_unreachable": "not_found",
  "scope.source_unreadable": "permission_denied",
  "scope.object_read_only": "permission_denied",
  "scope.manifest_missing": "invalid_data",
  "scope.manifest_ambiguous": "invalid_data",
  "scope.manifest_invalid": "invalid_data",
  "scope.source_invalid": "invalid_data",
  "scope.definition_invalid": "invalid_data",
  "scope.entity_id_invalid": "invalid_data",
  "scope.import_unresolved": "invalid_data",
  "scope.export_unresolved": "invalid_data",
  "scope.relation_invalid": "invalid_data",
  "scope.relation_start_unresolved": "invalid_data",
  "scope.relation_end_unresolved": "invalid_data",
  "scope.package_invalid": "invalid_data",
  "scope.dependency_unresolved": "invalid_data",
  "scope.mutation_invalid": "invalid_data",
  "scope.entity_duplicate": "conflict",
  "scope.relation_duplicate": "conflict",
  "scope.source_identity_conflict": "conflict",
  "scope.mutation_conflict": "conflict",
  "scope.mutation_write_failed": "internal",
  "scope.internal": "internal"
} as const satisfies Record<string, ErrorCode>;

export type ErrorReason = keyof typeof ERROR_REASON_CODES;

export class ApplicationError extends Error {
  readonly code: ErrorCode;
  readonly reason: ErrorReason;
  readonly details?: Readonly<Record<string, string>>;

  constructor(reason: ErrorReason, message: string, details?: Record<string, string>, cause?: unknown) {
    super(message, { cause });
    if (!Object.hasOwn(ERROR_REASON_CODES, reason)) throw new TypeError("unknown application error reason");
    this.name = "ApplicationError";
    this.code = ERROR_REASON_CODES[reason];
    this.reason = reason;
    if (details && Object.keys(details).length > 0) this.details = Object.freeze({ ...details });
  }
}

export function createError(
  reason: ErrorReason,
  message: string,
  details?: Record<string, string>,
  cause?: unknown
): ApplicationError {
  return new ApplicationError(reason, message, details, cause);
}

export function sourceError(error: unknown, message: string, details?: Record<string, string>): ApplicationError {
  if (error instanceof ApplicationError) return error;
  const code = typeof error === "object" && error !== null && "code" in error ? error.code : undefined;
  const reason =
    code === "ENOENT"
      ? "scope.source_not_found"
      : code === "EACCES" || code === "EPERM"
        ? "scope.source_unreadable"
        : "scope.internal";
  return createError(reason, message, details, error);
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export function normalizeError(error: unknown, operation: string): ApplicationError {
  if (error instanceof ApplicationError) return error;
  return createError("scope.internal", "scope operation failed", { operation }, error);
}
