import { invalidSession } from "~/lib/auth"
import { normalizeEntriesQuery } from "~/lib/entry-query"
import { api, ApiError } from "~/openapi/client"

import type { Route } from "./+types/files-index"

export async function clientLoader({ request, url }: Route.ClientLoaderArgs) {
  console.log(url)
  const query = normalizeEntriesQuery(request)
  const { data, error, response } = await api.GET("/api/v1/entries", {
    params: { query },
  })

  invalidSession(request, response)

  if (!response.ok) {
    throw new ApiError(response.status, error)
  }

  return data
}

export default function Files({ loaderData }: Route.ComponentProps) {
  return <div>Files</div>
}
