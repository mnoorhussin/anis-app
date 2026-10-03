/**
 * Turn a PocketBase error into something readable.
 *
 * Our routes put the cases a person can act on — a plan ceiling, "this
 * invitation is for another address" — in a plain `error` field, which the SDK
 * does not copy into `message`. Reading only `message` shows its generic
 * fallback instead of the reason.
 */
export function describeError(err: unknown): string {
  const res = (err as { response?: { error?: string; message?: string } })?.response;
  return res?.error ?? res?.message ?? (err instanceof Error ? err.message : String(err));
}

/** The HTTP status of a PocketBase error, or 0 if it has none. */
export function errorStatus(err: unknown): number {
  const s = (err as { status?: unknown })?.status;
  return typeof s === 'number' ? s : 0;
}
