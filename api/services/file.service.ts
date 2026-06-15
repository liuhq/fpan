import { and, eq, isNotNull } from "drizzle-orm"

import type { DbClient } from "#db/client.ts"
import { blobTable, fileTable } from "#db/schema.ts"

export type InsertFile = typeof fileTable.$inferInsert
export type SelectFile = typeof fileTable.$inferSelect

export const createFileService = (db: DbClient) =>
  ({
    create: async (file: InsertFile) => {
      return await db.insert(fileTable).values(file).returning()
    },
    softDelete: async (id: SelectFile["id"]) => {
      return await db.update(fileTable).set({ deleted_at: new Date() }).where(eq(fileTable.id, id))
    },
    hardDelete: async (id: SelectFile["id"]) => {
      return await db
        .delete(fileTable)
        .where(and(eq(fileTable.id, id), isNotNull(fileTable.deleted_at)))
        .returning()
    },
    updateName: async (id: SelectFile["id"], name: SelectFile["name"]) => {
      return await db.update(fileTable).set({ name }).where(eq(fileTable.id, id))
    },
    move: async (id: SelectFile["id"], folder_id: SelectFile["folder_id"]) => {
      return await db.update(fileTable).set({ folder_id }).where(eq(fileTable.id, id))
    },
    findById: async (id: SelectFile["id"]) => {
      return await db.select().from(fileTable).where(eq(fileTable.id, id))
    },
    getBlob: async (id: SelectFile["id"]) => {
      return await db
        .select()
        .from(blobTable)
        .innerJoin(fileTable, eq(blobTable.sha256, fileTable.sha256))
        .where(eq(fileTable.id, id))
    },
  }) as const
