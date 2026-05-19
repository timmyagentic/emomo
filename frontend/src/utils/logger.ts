export type LogContext = Record<string, unknown>;

export function logError(message: string, context?: LogContext): void {
  if (context) {
    console.error(message, context);
    return;
  }
  console.error(message);
}

export function logWarn(message: string, context?: LogContext): void {
  if (context) {
    console.warn(message, context);
    return;
  }
  console.warn(message);
}
