import { isNotNull, isNull, sql } from "drizzle-orm"
import {
  check,
  uniqueIndex,
  index,
  integer,
  text,
  sqliteTable,
  type AnySQLiteColumn,
} from "drizzle-orm/sqlite-core"
import { nanoid } from "nanoid"

const now = () => new Date()
const genId = () => nanoid()

/**
 * Physical blob storage — content-addressed store. Identical content is stored
 * exactly once, keyed by its SHA-256 hash. The on-disk path is derived from the
 * hash via sharding (e.g. `ab/cd/abcdef...`). Logical files reference a blob
 * through {@link fileTable.sha256}; the {@link ref_count} tracks how many files
 * point here and is maintained by database triggers. A blob whose `ref_count`
 * reaches zero is reclaimed (physical file deleted) by an application-level GC.
 */
export const blobTable = sqliteTable(
  "blobs",
  {
    /** Primary key. The SHA-256 of the content as 64 lowercase hex chars. */
    sha256: text({ length: 64 }).primaryKey(),

    /** Size of the physical blob in bytes. */
    size: integer().notNull(),

    /** Reference count: number of {@link fileTable} rows pointing here. Maintained by triggers. */
    ref_count: integer().notNull().default(0),

    /** Creation timestamp. Set once on insert. */
    created_at: integer({ mode: "timestamp" }).notNull().$default(now),
  },
  (t) => [check("ref_count_non_negative", sql`${t.ref_count} >= 0`)],
)

/**
 * Folders — lightweight hierarchical nodes expressing ownership and path only.
 * A single fixed root row (`id = 'ROOT'`) is seeded; every top-level folder has
 * `parent_id = 'ROOT'`, so `parent_id` is never null except for the root itself.
 * Children are cascade-deleted with the parent on physical delete; soft deletion
 * is handled through {@link deleted_at} and must be cascaded in the application layer.
 */
export const folderTable = sqliteTable(
  "folders",
  {
    /** Primary key. A 21-character nanoid; the root uses the literal id `'ROOT'`. */
    id: text({ length: 21 }).primaryKey().$default(genId),

    /**
     * Parent folder id (self-referential FK forming the tree).
     * `null` only for the root row. Children are cascade-deleted with the parent;
     * updates to the referenced id are restricted.
     */
    parent_id: text().references((): AnySQLiteColumn => folderTable.id, {
      onDelete: "cascade",
      onUpdate: "restrict",
    }),

    /** Display name of the folder. Required. */
    name: text().notNull(),

    /** Creation timestamp. Set once on insert. */
    created_at: integer({ mode: "timestamp" }).notNull().$default(now),

    /** Last-modified timestamp. Set on insert and automatically refreshed on every update. */
    updated_at: integer({ mode: "timestamp" }).notNull().$default(now).$onUpdate(now),

    /** Soft-deletion timestamp. `null` means active; a value marks it as deleted. */
    deleted_at: integer({ mode: "timestamp" }),
  },
  (t) => [
    /** Enforces unique folder names within the same parent, ignoring soft-deleted entries. */
    uniqueIndex("idx_folders_name_in_parent").on(t.name, t.parent_id).where(isNull(t.deleted_at)),

    /** Speeds up listing active child folders of a parent (excludes soft-deleted entries). */
    index("idx_folders_parent_active").on(t.parent_id).where(isNull(t.deleted_at)),
  ],
)

/**
 * Files — logical files stored as a flat list but each owned by exactly one
 * folder (single ownership). The content lives in {@link blobTable}, referenced
 * by {@link sha256}; multiple files may share one blob (deduplication). Supports
 * soft deletion through {@link deleted_at}.
 */
export const fileTable = sqliteTable(
  "files",
  {
    /** Primary key. A 21-character nanoid; publicly never exposed (use share tokens instead). */
    id: text({ length: 21 }).primaryKey().$default(genId),

    /**
     * Owning folder id. Top-level files belong to the root (`'ROOT'`).
     * Children are cascade-deleted with the folder; updates to the id are restricted.
     */
    folder_id: text()
      .notNull()
      .references(() => folderTable.id, {
        onDelete: "cascade",
        onUpdate: "restrict",
      }),

    /** Display name of the file. Allowed to repeat across folders; unique within a folder. */
    name: text().notNull(),

    /** MIME type of the file (e.g. `"image/png"`). Required. */
    mime_type: text().notNull(),

    /**
     * Content pointer (SHA-256). FK into {@link blobTable}. Deletes are restricted
     * so a referenced blob can never be orphaned by accident; updates are restricted.
     */
    sha256: text({ length: 64 })
      .notNull()
      .references(() => blobTable.sha256, {
        onDelete: "restrict",
        onUpdate: "restrict",
      }),

    /** Creation timestamp. Set once on insert. */
    created_at: integer({ mode: "timestamp" }).notNull().$default(now),

    /** Last-modified timestamp. Set on insert and automatically refreshed on every update. */
    updated_at: integer({ mode: "timestamp" }).notNull().$default(now).$onUpdate(now),

    /** Soft-deletion timestamp. `null` means active; a value marks it as deleted. */
    deleted_at: integer({ mode: "timestamp" }),
  },
  (t) => [
    /** Enforces unique file names within the same folder, ignoring soft-deleted entries. */
    uniqueIndex("idx_files_name_in_folder").on(t.name, t.folder_id).where(isNull(t.deleted_at)),

    /** Speeds up listing active files of a folder (excludes soft-deleted entries). */
    index("idx_files_folder_active").on(t.folder_id).where(isNull(t.deleted_at)),

    /** Look up files by content (deduplication / reverse reference). */
    index("idx_files_sha256").on(t.sha256),

    /** Partial index over soft-deleted files, e.g. for trash views or GC scans. */
    index("idx_files_deleted").on(t.deleted_at).where(isNotNull(t.deleted_at)),
  ],
)

/**
 * Shares — public share links to individual files. Visitors access a file only
 * through the opaque {@link token}; the internal {@link fileTable.id}, the
 * {@link blobTable.sha256}, and physical paths are never exposed. A share may be
 * time-limited ({@link expires_at}), password-protected ({@link password_hash}),
 * download-capped ({@link download_limit}), or manually revoked ({@link revoked_at}).
 */
export const shareTable = sqliteTable(
  "shares",
  {
    /** Primary key. A 21-character nanoid used as the public token in URLs. */
    token: text({ length: 21 }).primaryKey().$default(genId),

    /** Target file id. Cascade-deleted with the file; updates to the id are restricted. */
    file_id: text()
      .notNull()
      .references(() => fileTable.id, {
        onDelete: "cascade",
        onUpdate: "restrict",
      }),

    /** Expiry timestamp. `null` means the link never expires. */
    expires_at: integer({ mode: "timestamp" }),

    /** Optional access password, stored hashed (never plaintext). `null` means no password. */
    password_hash: text(),

    /** Optional maximum number of downloads. `null` means unlimited. */
    download_limit: integer(),

    /** Number of downloads served so far. */
    download_count: integer().notNull().default(0),

    /** Manual revocation timestamp. `null` means active. */
    revoked_at: integer({ mode: "timestamp" }),

    /** Creation timestamp. Set once on insert. */
    created_at: integer({ mode: "timestamp" }).notNull().$default(now),
  },
  (t) => [
    /** Speeds up listing / revoking all shares of a given file. */
    index("idx_shares_file").on(t.file_id),

    /** Ensures the download count never exceeds the configured limit. */
    check(
      "share_download_limit_check",
      sql`${t.download_limit} IS NULL OR ${t.download_count} <= ${t.download_limit}`,
    ),
  ],
)
