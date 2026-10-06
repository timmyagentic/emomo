export class EmomoError extends Error {
  constructor(code, message, details = {}) {
    super(message);
    this.name = 'EmomoError';
    this.code = code;
    this.retryable = false;
    Object.assign(this, details);
  }
}

export function publicError(error) {
  if (error instanceof EmomoError) {
    const result = { code: error.code, message: error.message, retryable: error.retryable };
    for (const key of ['httpStatus', 'retryAfterSeconds']) {
      if (error[key] !== undefined) result[key] = error[key];
    }
    return result;
  }
  // Network/filesystem errors may contain credential-bearing URLs or paths.
  return { code: 'INTERNAL_ERROR', message: 'The operation failed. Check local permissions and configuration.', retryable: false };
}
