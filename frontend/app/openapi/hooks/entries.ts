import useSWR from "swr"

import { api, ApiError } from "../client"
import { apiKeys } from "../keys"
import type { EntriesQuery, NormalizedEntriesQuery, ParentId } from "../types"

export function normalizeEntriesQuery(query: EntriesQuery = {}): NormalizedEntriesQuery {
  return {
    page: query.page ?? 1,
    size: query.size ?? 100,
    sort: query.sort ?? "asc",
    sort_by: query.sort_by ?? "name",
    ...(query.filter ? { filter: query.filter } : {}),
    type: query.type ?? "all",
  }
}

async function listRootEntries(query: NormalizedEntriesQuery) {
  const { data, error, response } = await api.GET("/api/v1/entries", { params: { query } })

  if (error) {
    throw new ApiError(response.status, error)
  }

  return data
}

async function listFolderEntries(parentId: NonNullable<ParentId>, query: NormalizedEntriesQuery) {
  const { data, error, response } = await api.GET("/api/v1/folders/{id}/entries", {
    params: { path: { id: parentId }, query },
  })

  if (error) {
    throw new ApiError(response.status, error)
  }

  return data
}

export function useEntries(parentId: ParentId, query: EntriesQuery = {}) {
  const normalizedQuery = normalizeEntriesQuery(query)

  return useSWR(apiKeys.entries(parentId, normalizedQuery), ([, currentParentId, currentQuery]) =>
    currentParentId === null
      ? listRootEntries(currentQuery)
      : listFolderEntries(currentParentId, currentQuery),
  )
}
