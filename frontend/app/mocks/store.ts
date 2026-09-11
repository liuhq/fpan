import { fakerEN, fakerZH_CN, type Faker } from "@faker-js/faker"

import type { Entry, FileEntry, FolderEntry, Share } from "~/openapi/types"

export type Scenario = "normal" | "empty" | "edge" | "error"

type Mutable<T> = { -readonly [K in keyof T]: T[K] }
export type StoredFile = Mutable<FileEntry>
export type StoredFolder = Mutable<FolderEntry>
export type StoredShare = Mutable<Share> & { password: string | null }

const encoder = new TextEncoder()
const normalSeed = 20_260_911
const edgeSeed = 20_260_912
const generatedFrom = new Date("2025-01-01T00:00:00.000Z")
const generatedTo = new Date("2026-08-31T23:59:59.000Z")

const fileKinds = [
  { extension: "txt", mimeType: "text/plain" },
  { extension: "md", mimeType: "text/markdown" },
  { extension: "json", mimeType: "application/json" },
  { extension: "csv", mimeType: "text/csv" },
] as const

function unixSeconds(value: Date) {
  return Math.floor(value.getTime() / 1_000)
}

function timestamps(fake: Faker, deleted: boolean) {
  const createdAt = fake.date.between({ from: generatedFrom, to: generatedTo })
  const updatedAt = fake.date.between({ from: createdAt, to: generatedTo })
  return {
    createdAt: unixSeconds(createdAt),
    updatedAt: unixSeconds(updatedAt),
    deletedAt: deleted
      ? unixSeconds(fake.date.between({ from: updatedAt, to: generatedTo }))
      : null,
  }
}

function displayName(fake: Faker, extension?: string) {
  const name = fake.word.words({ count: { min: 1, max: 3 } }).replaceAll(/\s+/g, "-")
  return extension ? `${name}.${extension}` : name
}

function sha256(fake: Faker) {
  return fake.string.hexadecimal({ length: 64, casing: "lower", prefix: "" })
}

function folder(
  fake: Faker,
  id: number,
  parentId: number | null,
  options: { display?: string; deleted?: boolean } = {},
): StoredFolder {
  const time = timestamps(fake, options.deleted ?? false)
  return {
    type: "folder",
    id,
    display: options.display ?? displayName(fake),
    parent_id: parentId,
    created_at: time.createdAt,
    updated_at: time.updatedAt,
    deleted_at: time.deletedAt,
  }
}

function file(
  fake: Faker,
  id: number,
  parentId: number | null,
  options: { display?: string; deleted?: boolean; empty?: boolean } = {},
): { item: StoredFile; bytes: Uint8Array } {
  const kind = fake.helpers.arrayElement(fileKinds)
  const bytes = options.empty ? new Uint8Array() : encoder.encode(`${fake.lorem.paragraphs(2)}\n`)
  const time = timestamps(fake, options.deleted ?? false)
  const hash = options.empty
    ? "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
    : sha256(fake)
  return {
    item: {
      type: "file",
      id,
      display: options.display ?? displayName(fake, kind.extension),
      parent_id: parentId,
      mime_type: kind.mimeType,
      blob: { sha256: hash, size: bytes.byteLength, created_at: time.createdAt },
      created_at: time.createdAt,
      updated_at: time.updatedAt,
      deleted_at: time.deletedAt,
    },
    bytes,
  }
}

function share(
  fake: Faker,
  id: number,
  entryId: number,
  entryType: "file" | "folder",
  options: Partial<
    Pick<StoredShare, "token" | "password" | "expires_at" | "max_downloads" | "download_count">
  > = {},
): StoredShare {
  const password = options.password ?? null
  const time = timestamps(fake, false)
  return {
    id,
    entry_id: entryId,
    entry_type: entryType,
    token: options.token ?? fake.string.alphanumeric(32),
    password,
    has_password: password !== null,
    expires_at: options.expires_at ?? null,
    max_downloads: options.max_downloads ?? null,
    download_count: options.download_count ?? 0,
    created_at: time.createdAt,
    updated_at: time.updatedAt,
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
    fakerEN.seed(normalSeed)
    this.folders.push(
      folder(fakerEN, 1, null),
      folder(fakerEN, 2, null),
      folder(fakerEN, 3, 1),
      folder(fakerEN, 4, null, { deleted: true }),
      folder(fakerEN, 5, 4, { deleted: true }),
    )
    this.addFile(file(fakerEN, 1, null))
    this.addFile(file(fakerEN, 2, 2))
    this.addFile(file(fakerEN, 3, 3))
    this.addFile(file(fakerEN, 4, null, { deleted: true }))
    this.addFile(file(fakerEN, 5, 5, { deleted: true }))
    this.shares.push(
      share(fakerEN, 1, 1, "folder", { token: "documents-public-token" }),
      share(fakerEN, 2, 2, "file", { token: "photo-password-token", password: "fpan" }),
    )
  }

  private seedEdge() {
    fakerZH_CN.seed(edgeSeed)
    this.folders.push(
      folder(fakerZH_CN, 20, null, { display: `${"非常长的目录名称-".repeat(12)}📁` }),
    )
    for (let index = 0; index < 125; index++) {
      const id = 100 + index
      const generatedName = fakerZH_CN.commerce.productName().replaceAll(/\s+/g, "-")
      this.addFile(
        file(fakerZH_CN, id, null, {
          display: `边界文件-${String(index).padStart(3, "0")}-${generatedName}-${"x".repeat(index % 30)}.txt`,
          empty: index === 0,
        }),
      )
    }
    this.shares.push(
      share(fakerZH_CN, 20, 1, "folder", {
        token: "expired-share-token",
        expires_at: unixSeconds(
          fakerZH_CN.date.between({
            from: new Date("2024-01-01T00:00:00.000Z"),
            to: new Date("2024-12-31T23:59:59.000Z"),
          }),
        ),
      }),
      share(fakerZH_CN, 21, 1, "folder", {
        token: "exhausted-share-token",
        max_downloads: 1,
        download_count: 1,
      }),
    )
  }

  private addFile(value: { item: StoredFile; bytes: Uint8Array }) {
    this.files.push(value.item)
    this.blobs.set(value.item.blob.sha256, value.bytes)
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
