import type { NormalizedEntriesQuery } from "~/openapi/types"

const DEFAULT_VALUE = {
  page: 1,
  size: 100,
  sort: "asc",
  sort_by: "name",
  type: "all",
} as const

function normPage(s: string | null): NormalizedEntriesQuery["page"] {
  if (!s) return DEFAULT_VALUE.page
  const n = Number(s)
  return n >= 1 ? n : 1
}

function normSize(s: string | null): NormalizedEntriesQuery["size"] {
  if (!s) return DEFAULT_VALUE.size
  const n = Number(s)
  return Math.max(Math.min(n, DEFAULT_VALUE.size), 1)
}

function normSort(s: string | null): NormalizedEntriesQuery["sort"] {
  if (!s) return DEFAULT_VALUE.sort
  return s === "asc" || s === "desc" ? s : DEFAULT_VALUE.sort
}

function normSortBy(s: string | null): NormalizedEntriesQuery["sort_by"] {
  if (!s) return DEFAULT_VALUE.sort_by
  return s === "name" || s === "created_at" || s === "updated_at" ? s : DEFAULT_VALUE.sort_by
}

function normType(s: string | null): NormalizedEntriesQuery["type"] {
  if (!s) return DEFAULT_VALUE.type
  return s === "all" || s === "file" || s === "folder" ? s : DEFAULT_VALUE.type
}

export function normalizeEntriesQuery(request: Request): NormalizedEntriesQuery {
  const url = new URL(request.url)
  const sp = url.searchParams

  const page = normPage(sp.get("page"))
  const size = normSize(sp.get("size"))
  const sort = normSort(sp.get("sort"))
  const sort_by = normSortBy(sp.get("sort_by"))
  const filter = sp.get("filter")
  const type = normType(sp.get("type"))

  return {
    page,
    size,
    sort,
    sort_by,
    ...(filter ? { filter: filter } : {}),
    type,
  }
}
