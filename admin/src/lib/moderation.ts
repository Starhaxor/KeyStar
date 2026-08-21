export function moderationStatus(
  params: URLSearchParams,
  fallback: string
): string {
  return params.get("status") ?? fallback;
}

export function deviceBanExpiry(
  now: string,
  durationHours: number | null
): string | undefined {
  if (durationHours === null) return undefined;
  return new Date(new Date(now).getTime() + durationHours * 60 * 60 * 1000).toISOString();
}
