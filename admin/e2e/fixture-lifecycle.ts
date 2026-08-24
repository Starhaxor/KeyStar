export async function withProvisionedFixture<T>(
  seed: () => Promise<string>,
  reset: () => Promise<void>,
  provide: (fixture: T) => Promise<void>,
) {
  try {
    const serializedFixture = await seed();
    const fixture = JSON.parse(serializedFixture) as T;
    await provide(fixture);
  } finally {
    await reset();
  }
}
