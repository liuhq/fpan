import type { NormalizedEntriesQuery } from "./types"

export const apiKeys = {
  entries: (parentId: number | null, query: NormalizedEntriesQuery) =>
    ["entries", parentId, query] as const,
} as const
