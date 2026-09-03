import useSWR from "swr"

import { api, ApiError, apiKeys } from "../client"
import type { paths } from "../schema"

export type EntriesQuery = paths["/api/v1/entries"]["get"]["parameters"]["query"]

async function listRootEntries(query: EntriesQuery) {
  const { data, error, response } = await api.GET("/api/v1/entries", { params: { query } })

  if (error) {
    throw new ApiError(response.status, error)
  }

  return data
}

async function listFolderEntries(parentId: number, query: EntriesQuery) {
  const { data, error, response } = await api.GET("/api/v1/folders/{id}/entries", {
    params: { path: parentId, query },
  })

  if (error) {
    throw new ApiError(response.status, error)
  }

  return data
}

export function useEntries(parentId: number | null, query: EntriesQuery) {
  return useSWR(
    apiKeys.entries(parentId, query),
    ([, currentParentId, currentQuery]) =>
      currentParentId === null
        ? listRootEntries(currentQuery)
        : listFolderEntries(currentParentId, currentQuery),
    {
      keepPreviousData: true,
    },
  )
}
