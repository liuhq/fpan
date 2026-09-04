import type { FolderId, NormalizedEntriesQuery, ParentId } from "./types"

export const apiKeys = {
  entries: (parentId: ParentId, query: NormalizedEntriesQuery) =>
    ["entries", parentId, query] as const,
  folders: {
    detail: (id: FolderId) => ["folder", id] as const,
    create: (parentId: ParentId) => ["folder-mut", "create", parentId] as const,
    update: (id: FolderId) => ["folder-mut", "update", id] as const,
    delete: (id: FolderId) => ["folder-mut", "delete", id] as const,
  },
} as const

export function isEntriesKeyForParent(key: unknown, parentId: number | null): boolean {
  return Array.isArray(key) && key[0] === "entries" && key[1] === parentId
}
