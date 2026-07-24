export class NotFoundError extends Error {
  override name = "NotFoundError"
}

export class AccessDeniedError extends Error {
  override name = "AccessDeniedError"
}

export class ValidationError extends Error {
  override name = "ValidationError"
}
