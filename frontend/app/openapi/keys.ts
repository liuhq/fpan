import type { FileId, FolderId, NormalizedEntriesQuery, ParentId } from "./types"

export const apiKeys = {
  entries: (parentId: ParentId, query: NormalizedEntriesQuery) =>
    ["entries", parentId, query] as const,
  folders: {
    detail: (id: FolderId) => ["folder", id] as const,
    create: (parentId: ParentId) => ["folder-mut", "create", parentId] as const,
    update: (id: FolderId) => ["folder-mut", "update", id] as const,
    delete: (id: FolderId) => ["folder-mut", "delete", id] as const,
  },
  files: {
    detail: (id: FileId) => ["file", id] as const,
    create: (parentId: ParentId) => ["file-mut", "create", parentId] as const,
    update: (id: FileId) => ["file-mut", "update", id] as const,
    delete: (id: FileId) => ["file-mut", "delete", id] as const,
  },
} as const

type EntriesKey = ReturnType<typeof apiKeys.entries>
type FolderDetailKey = ReturnType<typeof apiKeys.folders.detail>
type FileDetailKey = ReturnType<typeof apiKeys.files.detail>

export function isEntriesKey(key: unknown): key is EntriesKey {
  return Array.isArray(key) && key[0] === "entries"
}

export function isEntriesKeyForParent(key: unknown, parentId: ParentId): boolean {
  return isEntriesKey(key) && key[1] === parentId
}

export function isFolderDetailKey(key: unknown): key is FolderDetailKey {
  return Array.isArray(key) && key[0] === "folder"
}

export function isFileDetailKey(key: unknown): key is FileDetailKey {
  return Array.isArray(key) && key[0] === "file"
}
