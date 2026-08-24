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

  it("loads the application selector without a stale application context", async () => {
    document.cookie = "keystar_application_id=app-2; Path=/";
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true, items: [] }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.applications();

    expect(fetchMock).toHaveBeenCalledWith("/v1/admin/applications", expect.objectContaining({
      cache: "no-store",
      headers: expect.not.objectContaining({ "X-KeyStar-App": expect.anything() }),
    }));
  });

  it("does not cache tenant-scoped table requests after application changes", async () => {
    document.cookie = "keystar_application_id=app-2; Path=/";
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true, items: [], total: 0, page: 1, page_size: 20 }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.users(1, "");

    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining("/v1/admin/users?"), expect.objectContaining({
      cache: "no-store",
      headers: expect.objectContaining({ "X-KeyStar-App": "app-2" }),
    }));
  });

  it("omits stale application context from lifecycle recovery calls", async () => {
    document.cookie = "keystar_application_id=disabled-app; Path=/";
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true, items: [] }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.me();
    await api.organizations();
    await api.transitionApplication("disabled-app", "active");

    for (const [, init] of fetchMock.mock.calls) {
      expect(init).toEqual(expect.objectContaining({
        headers: expect.not.objectContaining({ "X-KeyStar-App": expect.anything() }),
      }));
    }
  });

  it("loads onboarding progress for the explicit initialized application", async () => {
    document.cookie = "keystar_application_id=stale-app; Path=/";
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.onboardingProgress("selected-app");

    expect(fetchMock).toHaveBeenCalledWith("/v1/admin/onboarding/progress", expect.objectContaining({
      headers: expect.objectContaining({ "X-KeyStar-App": "selected-app" }),
    }));
  });
});
