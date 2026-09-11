import { redirect } from "react-router"

import { useMockLogin } from "~/hooks/mockLogin"
import { checkSession, parseLoginURL, safeReturnTo } from "~/lib/auth"

import type { Route } from "./+types/login"

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

  const { handleLogin, isLoggingIn, loginError } = useMockLogin(returnTo)

  return (
    <main>
      <a
        href={parseLoginURL(returnTo)}
        onClick={handleLogin}
        aria-disabled={isLoggingIn}
        aria-busy={isLoggingIn}
      >
        {isLoggingIn ? "Logging in..." : "Login"}
      </a>
      {loginError && <p role="alert">{loginError}</p>}
    </main>
  )
}
