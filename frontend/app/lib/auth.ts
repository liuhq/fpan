import { redirect } from "react-router"

import { api, ApiError } from "~/openapi/client"

export function parseReturnTo(request: Request): string {
  const url = new URL(request.url)
  const returnTo = `${url.pathname}${url.search}`
  return returnTo
}

function loginPath(request: Request) {
  const returnTo = parseReturnTo(request)
  const p = `/login?return_to=${encodeURIComponent(returnTo)}`
  return p
}

export async function checkSession(signal?: AbortSignal): Promise<boolean> {
  const { error, response } = await api.GET("/api/v1/auth/session", {
    signal,
    cache: "no-store",
  })

  if (response.status === 204) {
    return true
  }

  if (response.status === 401) {
    return false
  }

  throw new ApiError(response.status, error)
}

export async function requireSession(request: Request): Promise<void> {
  const isValid = await checkSession(request.signal)
  if (!isValid) {
    throw redirect(loginPath(request))
  }
}

export async function invalidSession(request: Request, response: Response) {
  if (response.status === 401) {
    throw redirect(loginPath(request))
  }
}
