import { NextResponse, type NextRequest } from "next/server";

// Route guard for the admin console. Protected pages require an active
// backend session, verified against GET /v1/admin/me. The check is
// fail-closed for authentication errors and backend outages so the console
// never renders until the session and MFA state can be verified.

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const SESSION_COOKIE = "starloader_admin_session";

function redirectToSignin(request: NextRequest) {
  const url = new URL("/signin", request.url);
  return NextResponse.redirect(url);
}

function redirectToSecurity(request: NextRequest) {
  return NextResponse.redirect(new URL("/security", request.url));
}

export default async function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl;
  const sessionToken = request.cookies.get(SESSION_COOKIE)?.value;

  if (pathname.startsWith("/signin")) {
    if (!sessionToken) {
      return NextResponse.next();
    }
    const state = await verifySession(request, sessionToken);
    if (state === "active") return NextResponse.redirect(new URL("/", request.url));
    if (state === "mfa_required") return redirectToSecurity(request);
    return NextResponse.next();
  }

  if (!sessionToken) {
    return redirectToSignin(request);
  }
  const state = await verifySession(request, sessionToken);
  if (state === false) {
    const response = redirectToSignin(request);
    response.cookies.delete(SESSION_COOKIE);
    response.cookies.delete("starloader_admin_csrf");
    return response;
  }
  if (state === null) {
    return redirectToSignin(request);
  }
  if (state === "mfa_required" && pathname !== "/security") {
    return redirectToSecurity(request);
  }
  return NextResponse.next();
}

// Distinguishes a complete session from the deliberately restricted session
// issued for mandatory first-login MFA enrollment.
async function verifySession(
  request: NextRequest,
  sessionToken: string
): Promise<"active" | "mfa_required" | false | null> {
  try {
    const response = await fetch(`${API_URL}/v1/admin/me`, {
      headers: { cookie: `${SESSION_COOKIE}=${sessionToken}` },
      cache: "no-store",
    });
    if (!response.ok) return false;
    const identity = (await response.json()) as { mfa_enrolled?: unknown };
    if (identity.mfa_enrolled === true) return "active";
    if (identity.mfa_enrolled === false) return "mfa_required";
    return false;
  } catch {
    return null;
  }
}

export const config = {
  // Guard pages only; /v1/* API calls fall through to the rewrites that
  // proxy them to the backend, where auth is enforced.
  matcher: ["/((?!_next/static|_next/image|favicon.ico|images/|v1/).*)"],
};
