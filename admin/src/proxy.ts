import { NextResponse, type NextRequest } from "next/server";

// Route guard for the admin console. Protected pages require an active
// backend session, verified against GET /v1/admin/me. The check is
// fail-closed for authentication errors but tolerant of backend outages:
// if the API is unreachable the page itself surfaces the failure.

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const SESSION_COOKIE = "starloader_admin_session";

function redirectToSignin(request: NextRequest) {
  const url = new URL("/signin", request.url);
  return NextResponse.redirect(url);
}

export default async function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl;
  const sessionToken = request.cookies.get(SESSION_COOKIE)?.value;

  if (pathname.startsWith("/signin")) {
    if (!sessionToken) {
      return NextResponse.next();
    }
    const verified = await verifySession(request, sessionToken);
    return verified ? NextResponse.redirect(new URL("/", request.url)) : NextResponse.next();
  }

  if (!sessionToken) {
    return redirectToSignin(request);
  }
  const verified = await verifySession(request, sessionToken);
  if (verified === false) {
    const response = redirectToSignin(request);
    response.cookies.delete(SESSION_COOKIE);
    response.cookies.delete("starloader_admin_csrf");
    return response;
  }
  return NextResponse.next();
}

// Returns true (active), false (rejected), or null (backend unreachable).
async function verifySession(
  request: NextRequest,
  sessionToken: string
): Promise<boolean | null> {
  try {
    const response = await fetch(`${API_URL}/v1/admin/me`, {
      headers: { cookie: `${SESSION_COOKIE}=${sessionToken}` },
      cache: "no-store",
    });
    return response.ok;
  } catch {
    return null;
  }
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico|images/).*)"],
};
