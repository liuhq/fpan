import { and, eq, inArray, isNotNull, isNull } from "drizzle-orm"

import type { DbClient } from "#db/client.ts"
import { fileTable, folderTable } from "#db/schema.ts"
import type { SelectFile } from "#services/file.service.ts"

export type InsertFolder = typeof folderTable.$inferInsert
export type SelectFolder = typeof folderTable.$inferSelect

export const createFolderService = (db: DbClient) => {
  const getChildren = async (
    id: SelectFolder["id"],
    opts?: { limit?: number; offset?: number },
  ) => {
    const baseFilesQuery = db
      .select()
      .from(fileTable)
      .where(and(eq(fileTable.folder_id, id), isNull(fileTable.deleted_at)))
    const baseFoldersQuery = db
      .select()
      .from(folderTable)
      .where(and(eq(folderTable.parent_id, id), isNull(folderTable.deleted_at)))

    const [files, folders] = await Promise.all([
      opts?.limit != null || opts?.offset != null
        ? baseFilesQuery.limit(opts.limit!).offset(opts.offset!)
        : baseFilesQuery,
      opts?.limit != null || opts?.offset != null
        ? baseFoldersQuery.limit(opts.limit!).offset(opts.offset!)
        : baseFoldersQuery,
    ])
    return { files, folders }
  }
  return {
    create: async (folder: InsertFolder) => {
      return await db.insert(folderTable).values(folder).returning()
    },
    softDelete: async (id: SelectFolder["id"]) => {
      const recCollect = async (
        innerId: SelectFolder["id"],
      ): Promise<{ folders: SelectFolder["id"][]; files: SelectFile["id"][] }> => {
        const children = await getChildren(innerId)
        const folders = children.folders.map((f) => f.id)
        const files = children.files.map((f) => f.id)

        const sub = await Promise.all(folders.map(async (f) => await recCollect(f)))

        return sub.reduce(
          (acc, s) => ({
            folders: acc.folders.concat(s.folders),
            files: acc.files.concat(s.files),
          }),
          { folders, files },
        )
      }

      const now = new Date()
      const target = await recCollect(id)

      const [folders, files] = await db.batch([
        db
          .update(folderTable)
          .set({ deleted_at: now })
          .where(inArray(folderTable.id, target.folders.concat([id])))
          .returning(),
        db
          .update(fileTable)
          .set({ deleted_at: now })
          .where(inArray(fileTable.id, target.files))
          .returning(),
      ])
      return { folders, files }
    },
    hardDelete: async (id: SelectFolder["id"]) => {
      return await db
        .delete(folderTable)
        .where(and(eq(folderTable.id, id), isNotNull(folderTable.deleted_at)))
        .returning()
    },
    updateName: async (id: SelectFolder["id"], name: SelectFolder["name"]) => {
      return await db.update(folderTable).set({ name }).where(eq(folderTable.id, id)).returning()
    },
    move: async (id: SelectFolder["id"], parent_id: SelectFolder["parent_id"]) => {
      return await db
        .update(folderTable)
        .set({ parent_id })
        .where(eq(folderTable.id, id))
        .returning()
    },
    findById: async (id: SelectFolder["id"]) => {
      return await db.select().from(folderTable).where(eq(folderTable.id, id))
    },
    getChildren,
  } as const
}
