import { Outlet } from "react-router"

import Header from "~/components/header"
import { requireSession } from "~/lib/auth"

import type { Route } from "./+types/authenticated"

const authMiddleware: Route.ClientMiddlewareFunction = async ({ request }, next) => {
  await requireSession(request)
  await next()
}

export const clientMiddleware = [authMiddleware]

export default function Authenticated() {
  return (
    <>
      <main>
        <Header />
        <Outlet />
      </main>
    </>
  )
}
