import type { DBError } from './types'

export type ErrorCode =
  | 'validation'
  | 'not_found'
  | 'not_connected'
  | 'password_required'
  | 'ssh_secret_required'
  | 'ssh_host_unknown'
  | 'keyring_unavailable'
  | 'database'
  | 'confirmation_required'
  | 'read_only'
  | 'busy'
  | 'cancelled'
  | 'internal'

/** Error raised by a backend call, decoded from the Go apperr.Error JSON. */
export class AppError extends Error {
  readonly code: ErrorCode
  readonly db?: DBError
  readonly details?: unknown

  constructor(code: ErrorCode, message: string, db?: DBError, details?: unknown) {
    super(message)
    this.name = 'AppError'
    this.code = code
    this.db = db
    this.details = details
  }
}

/** Converts whatever a Wails promise rejected with into an AppError. */
export function toAppError(err: unknown): AppError {
  if (err instanceof AppError) return err
  const raw = typeof err === 'string' ? err : err instanceof Error ? err.message : String(err)
  try {
    const parsed = JSON.parse(raw) as { code?: ErrorCode; message?: string; db?: DBError; details?: unknown }
    if (parsed && typeof parsed === 'object' && parsed.code && parsed.message !== undefined) {
      return new AppError(parsed.code, parsed.message, parsed.db, parsed.details)
    }
  } catch {
    // not JSON: plain error text
  }
  return new AppError('internal', raw)
}

export function errorMessage(err: unknown): string {
  return toAppError(err).message
}
