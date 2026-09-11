import { startTransition, StrictMode } from "react"
import { hydrateRoot } from "react-dom/client"
import { HydratedRouter } from "react-router/dom"

async function enableMocking() {
  if (!import.meta.env.DEV || import.meta.env.MODE !== "mock") {
    return
  }

  const { worker } = await import("~/mocks/browser")
  await worker.start({
    onUnhandledRequest(request, print) {
      const pathname = new URL(request.url).pathname
      if (pathname === "/healthz" || pathname === "/readyz" || pathname.startsWith("/api/")) {
        print.error()
      }
    },
  })
}

await enableMocking()

startTransition(() => {
  hydrateRoot(
    document,
    <StrictMode>
      <HydratedRouter />
    </StrictMode>,
  )
})
