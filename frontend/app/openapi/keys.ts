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
  trash: {
    detail: () => ["trash"] as const,
    delete: () => ["trash-mut", "delete"] as const,
    restore: () => ["trash-mut", "restore"] as const,
  },
  auth: {
    logout: () => ["auth-mut", "logout"] as const,
  },
} as const

export function isEntriesKey(key: unknown): key is ReturnType<typeof apiKeys.entries> {
  return Array.isArray(key) && key[0] === "entries"
}

export function isEntriesKeyForParent(key: unknown, parentId: ParentId): boolean {
  return isEntriesKey(key) && key[1] === parentId
}

export function isFolderDetailKey(key: unknown): key is ReturnType<typeof apiKeys.folders.detail> {
  return Array.isArray(key) && key[0] === "folder"
}

export function isFileDetailKey(key: unknown): key is ReturnType<typeof apiKeys.files.detail> {
  return Array.isArray(key) && key[0] === "file"
}

export function isTrashDetailKey(key: unknown): key is ReturnType<typeof apiKeys.trash.detail> {
  return Array.isArray(key) && key[0] === "trash"
}
