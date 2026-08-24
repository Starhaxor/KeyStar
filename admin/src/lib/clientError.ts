export function reportClientError(error: unknown, message: string) {
  console.error(message, error);
  return message;
}
