export class DomainError extends Error {
  constructor(
    readonly code: string,
    message: string,
    readonly exitCode: number = 1,
    readonly retryable: boolean = false,
    readonly details?: Record<string, unknown>,
  ) { super(message); this.name = 'DomainError'; }
  publicValue() {
    return {code: this.code, message: this.message, retryable: this.retryable,
      ...(this.details ? {details: this.details} : {})};
  }
}
export function usage(message: string, details?: Record<string, unknown>): never {
  throw new DomainError('USAGE_ERROR', message, 2, false, details);
}
export function asDomainError(error: unknown): DomainError {
  return error instanceof DomainError ? error : new DomainError('INTERNAL_ERROR', 'The operation failed unexpectedly');
}
