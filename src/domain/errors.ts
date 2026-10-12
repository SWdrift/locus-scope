export type ErrorCode = 'invalid_argument' | 'invalid_data' | 'not_found' | 'conflict' | 'permission_denied' | 'internal';

export class ApplicationError extends Error {
  readonly code: ErrorCode;
  readonly reason: string;
  readonly details?: Record<string, string>;

  constructor(code: ErrorCode, reason: string, message: string, details?: Record<string, string>, cause?: unknown) {
    super(message, { cause });
    this.name = 'ApplicationError';
    this.code = code;
    this.reason = reason;
    if (details && Object.keys(details).length > 0) this.details = { ...details };
  }
}

export class ScopeError extends Error {
  override readonly name = 'ScopeError';

  constructor(message: string, options?: ErrorOptions) {
    super(message, options);
  }
}

export function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message;
  return String(error);
}

export function invalidArgument(reason: string, message: string, cause?: unknown): ApplicationError {
  return new ApplicationError('invalid_argument', reason, message, undefined, cause);
}
export function notFound(reason: string, message: string, cause?: unknown, details?: Record<string, string>): ApplicationError {
  return new ApplicationError('not_found', reason, message, details, cause);
}

export function permissionDenied(reason: string, message: string, cause?: unknown): ApplicationError {
  return new ApplicationError('permission_denied', reason, message, undefined, cause);
}

export function normalizeError(error: unknown, operation: string): ApplicationError {
  if (error instanceof ApplicationError) return error;
  return new ApplicationError('internal', 'scope.internal', errorMessage(error), { operation }, error);
}
