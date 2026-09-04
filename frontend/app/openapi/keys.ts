import type { NormalizedEntriesQuery } from "./types"

export const apiKeys = {
  entries: (parentId: number | null, query: NormalizedEntriesQuery) =>
    ["entries", parentId, query] as const,
} as const

export function isEntriesKeyForParent(key: unknown, parentId: number | null): boolean {
  return Array.isArray(key) && key[0] === "entries" && key[1] === parentId
}
