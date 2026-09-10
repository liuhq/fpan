import { redirect } from "react-router"

import { checkSession } from "~/auth"

import type { Route } from "./+types/login"

function parseLoginURL(returnTo: string) {
  const LOGIN_URL = "/api/v1/auth/login"
  return `${LOGIN_URL}?return_to=${encodeURIComponent(returnTo)}`
}

function safeReturnTo(p: string | null, origin: string): string {
  if (!p || !p.startsWith("/") || p.startsWith("//") || p === "/login" || p === "/logout") {
    return "/"
  }

  try {
    const target = new URL(p, origin) // Parse error: TypeError

    // prevent address injection
    if (target.origin !== origin) {
      return "/"
    }

    return `${target.pathname}${target.search}${target.hash}`
  } catch {
    return "/"
  }
}

export async function clientLoader({ request }: Route.ClientLoaderArgs) {
  const url = new URL(request.url)
  const returnTo = safeReturnTo(url.searchParams.get("return_to"), url.origin)
  const isValid = await checkSession(request.signal)

  if (isValid) {
    throw redirect(returnTo)
  }

  return { returnTo }
}

export default function Login({ loaderData }: Route.ComponentProps) {
  const { returnTo } = loaderData

  return (
    <main>
      <a href={parseLoginURL(returnTo)}>Login</a>
    </main>
  )
}
