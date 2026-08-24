import { afterEach, describe, expect, it, vi } from "vitest";
import { reportClientError } from "./clientError";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("reportClientError", () => {
  it("returns action-safe copy instead of a server failure message", () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);

    const message = reportClientError(
      new Error("postgres://operator:secret@10.0.0.12/keystar"),
      "Unable to load applications. Try again."
    );

    expect(message).toBe("Unable to load applications. Try again.");
    expect(message).not.toContain("postgres://");
  });
});
