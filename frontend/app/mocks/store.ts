import type { Entry, FileEntry, FolderEntry, Share } from "~/openapi/types"

export type Scenario = "normal" | "empty" | "edge" | "error"

type Mutable<T> = { -readonly [K in keyof T]: T[K] }
export type StoredFile = Mutable<FileEntry>
export type StoredFolder = Mutable<FolderEntry>
export type StoredShare = Mutable<Share> & { password: string | null }

const baseTime = 1_788_800_000
const encoder = new TextEncoder()

function folder(
  id: number,
  display: string,
  parentId: number | null,
  deletedAt: number | null = null,
): StoredFolder {
  return {
    type: "folder",
    id,
    display,
    parent_id: parentId,
    created_at: baseTime + id * 100,
    updated_at: baseTime + id * 100,
    deleted_at: deletedAt,
  }
}

function file(
  id: number,
  display: string,
  parentId: number | null,
  mimeType: string,
  hash: string,
  size: number,
  deletedAt: number | null = null,
): StoredFile {
  return {
    type: "file",
    id,
    display,
    parent_id: parentId,
    mime_type: mimeType,
    blob: { sha256: hash, size, created_at: baseTime + id * 100 },
    created_at: baseTime + id * 100,
    updated_at: baseTime + id * 100,
    deleted_at: deletedAt,
  }
}

function share(
  id: number,
  entryId: number,
  entryType: "file" | "folder",
  token: string,
  options: Partial<
    Pick<StoredShare, "password" | "expires_at" | "max_downloads" | "download_count">
  > = {},
): StoredShare {
  const password = options.password ?? null
  return {
    id,
    entry_id: entryId,
    entry_type: entryType,
    token,
    password,
    has_password: password !== null,
    expires_at: options.expires_at ?? null,
    max_downloads: options.max_downloads ?? null,
    download_count: options.download_count ?? 0,
    created_at: baseTime + id * 100,
    updated_at: baseTime + id * 100,
  }
}

export function scenarioFromLocation(): Scenario {
  const value = new URLSearchParams(window.location.search).get("mock")
  return value === "empty" || value === "edge" || value === "error" ? value : "normal"
}

export class MockStore {
  files: StoredFile[] = []
  folders: StoredFolder[] = []
  shares: StoredShare[] = []
  blobs = new Map<string, Uint8Array>()
  nextFileId = 1
  nextFolderId = 1
  nextShareId = 1

  constructor(readonly scenario: Scenario) {
    if (scenario !== "empty") {
      this.seedNormal()
    }
    if (scenario === "edge") {
      this.seedEdge()
    }
    this.refreshIds()
  }

  private seedNormal() {
    const contents = [
      ["279d0751ac66d11a7d4f730d55e71d00f3b031de3746844af71b321f4c79604c", "Welcome to Fpan\n"],
      ["e1d00855674b391bb1b3e5f03b18cd3a23d98b41a508d4d485a6c35f635968e2", "mock image bytes"],
      ["ce6bd5f11c19eed6adc18a120553f750d46ff00570e891792780f312905e30a2", "Quarterly plan\n"],
      ["9bc82f53a4c4501e3140bb08b63ae160e77b690a557193841324b7dd9d92d2f7", "deleted note\n"],
      ["4a44d8f571d5afabe082ad7fd12c332f785d256d74fffa9415f1febfdf09c271", "archived child\n"],
    ] as const
    for (const [hash, value] of contents) this.blobs.set(hash, encoder.encode(value))
    this.folders.push(
      folder(1, "Documents", null),
      folder(2, "Photos", null),
      folder(3, "Projects", 1),
      folder(4, "Archived", null, baseTime + 9_000),
      folder(5, "Archive child", 4, baseTime + 9_000),
    )
    this.files.push(
      file(1, "README.txt", null, "text/plain", contents[0][0], contents[0][1].length),
      file(2, "sunset.jpg", 2, "image/jpeg", contents[1][0], contents[1][1].length),
      file(3, "plan.md", 3, "text/markdown", contents[2][0], contents[2][1].length),
      file(
        4,
        "deleted.txt",
        null,
        "text/plain",
        contents[3][0],
        contents[3][1].length,
        baseTime + 9_100,
      ),
      file(
        5,
        "inside-archive.txt",
        5,
        "text/plain",
        contents[4][0],
        contents[4][1].length,
        baseTime + 9_000,
      ),
    )
    this.shares.push(
      share(1, 1, "folder", "documents-public-token"),
      share(2, 2, "file", "photo-password-token", { password: "fpan" }),
    )
  }

  private seedEdge() {
    this.folders.push(folder(20, `${"非常长的目录名称-".repeat(12)}📁`, null))
    const emptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
    const contentHash = "e1d00855674b391bb1b3e5f03b18cd3a23d98b41a508d4d485a6c35f635968e2"
    this.blobs.set(emptyHash, new Uint8Array())
    for (let index = 0; index < 125; index++) {
      const id = 100 + index
      const hash = index === 0 ? emptyHash : contentHash
      const bytes = index === 0 ? new Uint8Array() : encoder.encode("mock image bytes")
      this.blobs.set(hash, bytes)
      this.files.push(
        file(
          id,
          `边界文件-${String(index).padStart(3, "0")}-${"x".repeat(index % 30)}.txt`,
          null,
          "text/plain",
          hash,
          bytes.byteLength,
        ),
      )
    }
    this.shares.push(
      share(20, 1, "folder", "expired-share-token", { expires_at: baseTime - 1 }),
      share(21, 1, "folder", "exhausted-share-token", { max_downloads: 1, download_count: 1 }),
    )
  }

  private refreshIds() {
    this.nextFileId = Math.max(0, ...this.files.map(({ id }) => id)) + 1
    this.nextFolderId = Math.max(0, ...this.folders.map(({ id }) => id)) + 1
    this.nextShareId = Math.max(0, ...this.shares.map(({ id }) => id)) + 1
  }

  activeFolder(id: number) {
    return this.folders.find((item) => item.id === id && item.deleted_at === null)
  }

  activeFile(id: number) {
    return this.files.find((item) => item.id === id && item.deleted_at === null)
  }

  activeEntry(type: "file" | "folder", id: number): Entry | undefined {
    return type === "file" ? this.activeFile(id) : this.activeFolder(id)
  }

  entries(parentId: number | null) {
    return [...this.folders, ...this.files].filter(
      (item) => item.deleted_at === null && item.parent_id === parentId,
    ) as Entry[]
  }

  descendantFolderIds(id: number) {
    const ids = new Set([id])
    let changed = true
    while (changed) {
      changed = false
      for (const item of this.folders) {
        if (item.parent_id !== null && ids.has(item.parent_id) && !ids.has(item.id)) {
          ids.add(item.id)
          changed = true
        }
      }
    }
    return ids
  }

  siblingNameExists(
    display: string,
    parentId: number | null,
    except?: { type: Entry["type"]; id: number },
  ) {
    return [...this.folders, ...this.files].some(
      (item) =>
        item.deleted_at === null &&
        item.parent_id === parentId &&
        item.display === display &&
        (item.type !== except?.type || item.id !== except.id),
    )
  }

  trash() {
    return [...this.folders, ...this.files].filter((item) => {
      if (item.deleted_at === null) return false
      return item.parent_id === null || this.activeFolder(item.parent_id) !== undefined
    }) as Entry[]
  }

  purge(type: "file" | "folder", id: number) {
    if (type === "file") this.files = this.files.filter((item) => item.id !== id)
    else {
      const ids = this.descendantFolderIds(id)
      this.folders = this.folders.filter((item) => !ids.has(item.id))
      this.files = this.files.filter((item) => item.parent_id === null || !ids.has(item.parent_id))
    }
    this.collectBlobs()
  }

  collectBlobs() {
    const referenced = new Set(this.files.map((item) => item.blob.sha256))
    for (const hash of this.blobs.keys()) if (!referenced.has(hash)) this.blobs.delete(hash)
  }
}

export const store = new MockStore(scenarioFromLocation())
