import type { NormalizedEntriesQuery, ParentId } from "./types"

export const apiKeys = {
  entries: (parentId: ParentId, query: NormalizedEntriesQuery) =>
    ["entries", parentId, query] as const,
  folders: (opt: string, id?: ParentId) => ["folders", opt, id] as const,
} as const

export function isEntriesKeyForParent(key: unknown, parentId: number | null): boolean {
  return Array.isArray(key) && key[0] === "entries" && key[1] === parentId
}
