import { Outlet } from "react-router"

import { requireSession } from "~/auth"

import type { Route } from "./+types/authenticated"
import Header from "./header"

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
