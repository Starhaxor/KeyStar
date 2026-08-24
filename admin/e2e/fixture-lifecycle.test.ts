import { describe, expect, it, vi } from "vitest";
import { withProvisionedFixture } from "./fixture-lifecycle";

describe("withProvisionedFixture", () => {
  it("resets the database when seeding fails after a partial write", async () => {
    const reset = vi.fn().mockResolvedValue(undefined);

    await expect(
      withProvisionedFixture(
        vi.fn().mockRejectedValue(new Error("partial seed")),
        reset,
        vi.fn(),
      ),
    ).rejects.toThrow("partial seed");

    expect(reset).toHaveBeenCalledOnce();
  });

  it("resets the database when fixture JSON cannot be parsed", async () => {
    const reset = vi.fn().mockResolvedValue(undefined);

    await expect(
      withProvisionedFixture(
        vi.fn().mockResolvedValue("not-json"),
        reset,
        vi.fn(),
      ),
    ).rejects.toThrow();

    expect(reset).toHaveBeenCalledOnce();
  });
});
