import type { EntriesQuery } from "./hooks/entries"

export const apiKeys = {
  entries: (parentId: number | null, query: EntriesQuery) => ["entries", parentId, query] as const,
} as const
