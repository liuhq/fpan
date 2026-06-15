import { and, eq, isNull } from "drizzle-orm"

import type { DbClient } from "#db/client.ts"
import { fileTable, folderTable } from "#db/schema.ts"

export type InsertFolder = typeof folderTable.$inferInsert
export type SelectFolder = typeof folderTable.$inferSelect

export const createFolderService = (db: DbClient) =>
  ({
    create: async (folder: InsertFolder) => {
      return await db.insert(folderTable).values(folder).returning()
    },
    softDelete: async (id: SelectFolder["id"]) => {
      return await db
        .update(folderTable)
        .set({ deleted_at: new Date() })
        .where(eq(folderTable.id, id))
    },
    updateName: async (id: SelectFolder["id"], name: SelectFolder["name"]) => {
      return await db.update(folderTable).set({ name }).where(eq(folderTable.id, id))
    },
    move: async (id: SelectFolder["id"], parent_id: SelectFolder["parent_id"]) => {
      return await db.update(folderTable).set({ parent_id }).where(eq(folderTable.id, id))
    },
    findById: async (id: SelectFolder["id"]) => {
      return await db.select().from(folderTable).where(eq(folderTable.id, id))
    },
    getChildren: async (id: SelectFolder["id"]) => {
      const [files, folders] = await Promise.all([
        db
          .select()
          .from(fileTable)
          .where(and(eq(fileTable.folder_id, id), isNull(fileTable.deleted_at))),
        db
          .select()
          .from(folderTable)
          .where(and(eq(folderTable.parent_id, id), isNull(folderTable.deleted_at))),
      ])
      return { files, folders }
    },
  }) as const
