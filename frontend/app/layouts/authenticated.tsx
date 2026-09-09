import { Outlet } from "react-router"

import type { Route } from "./+types/authenticated"

export async function clientLoader() {}

export default function Authenticated() {
  return (
    <>
      <main>
        <Outlet />
      </main>
    </>
  )
}
