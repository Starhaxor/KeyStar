import { NextRequest } from "next/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import proxy from "./proxy";

function request(pathname: string) {
  return new NextRequest(`http://localhost:3000${pathname}`, {
    headers: { cookie: "starloader_admin_session=session-token" },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("admin page guard", () => {
  it("redirects an unenrolled administrator to mandatory MFA setup", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true, mfa_enrolled: false }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    })));

    const response = await proxy(request("/"));

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe("http://localhost:3000/security");
  });

  it("allows an unenrolled administrator to reach MFA setup itself", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true, mfa_enrolled: false }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    })));

    const response = await proxy(request("/security"));

    expect(response.headers.get("x-middleware-next")).toBe("1");
  });

  it("does not confuse security events with the MFA enrollment route", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true, mfa_enrolled: false }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    })));

    const response = await proxy(request("/security-events"));

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe("http://localhost:3000/security");
  });

  it("fails closed when MFA state cannot be verified", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("backend offline")));

    const response = await proxy(request("/"));

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe("http://localhost:3000/signin");
  });
});
