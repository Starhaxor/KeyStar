import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "./api";

describe("admin API client", () => {
  afterEach(() => { vi.unstubAllGlobals(); document.cookie = "keystar_application_id=; Max-Age=0; Path=/"; });

  it("sends the selected application header for webhook creation", async () => {
    document.cookie = "keystar_application_id=app-2; Path=/";
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true, webhook: { id: "webhook-1" }, secret: "once" }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    await api.createWebhook({ url: "https://example.test/hook", events: ["license.created"] });
    expect(fetchMock).toHaveBeenCalledWith("/v1/admin/webhooks", expect.objectContaining({ headers: expect.objectContaining({ "X-KeyStar-App": "app-2", "Content-Type": "application/json" }) }));
  });
});
