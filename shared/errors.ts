// errorDetail turns a caught value into the text worth showing a user
// under a localized headline — e.g. the RPC client's "DescribeAgent is
// not supported by BRUV server dev-f413920 — update the server…". The
// headline says what failed; this says why, so a failure is diagnosable
// from a screenshot instead of a console log.
export function errorDetail(e: unknown): string {
  if (e instanceof Error) return e.message
  if (typeof e === 'string') return e
  return ''
}
