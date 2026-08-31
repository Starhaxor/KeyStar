import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "./api";
import type { ApplicationSigningKeyMetadata } from "./types";

describe("application signing-key admin API client", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "keystar_application_id=; Max-Age=0; Path=/";
  });

  it("loads public metadata for the named application without a stale application header", async () => {
    document.cookie = "keystar_application_id=disabled-default; Path=/";
    const metadata = {
      kid: "ksk_AAAAAAAAAAAAAAAAAAAAAA",
      algorithm: "Ed25519" as const,
      status: "active" as const,
      public_key: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
      created_at: "2026-08-31T12:00:00Z",
      activated_at: "2026-08-31T12:00:00Z",
      retire_at: null,
      revoked_at: null,
    } satisfies ApplicationSigningKeyMetadata;
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: true, items: [metadata] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    );
    vi.stubGlobal("fetch", fetchMock);

    const response = await api.applicationSigningKeys("target-application");

    expect(response).toEqual({ ok: true, items: [metadata] });
    expect(fetchMock).toHaveBeenCalledWith(
      "/v1/admin/applications/target-application/signing-keys",
      expect.objectContaining({
        method: "GET",
        headers: expect.not.objectContaining({
          "X-KeyStar-App": expect.anything(),
        }),
      })
    );
  });
});
