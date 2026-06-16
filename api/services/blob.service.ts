import { and, eq } from "drizzle-orm"

import type { DbClient } from "#db/client.ts"
import { blobTable } from "#db/schema.ts"

export type InsertBlob = typeof blobTable.$inferInsert
export type SelectBlob = typeof blobTable.$inferSelect

export const createBlobService = (db: DbClient) =>
  ({
    prune: async (sha256: SelectBlob["sha256"]) => {
      return await db
        .delete(blobTable)
        .where(and(eq(blobTable.sha256, sha256), eq(blobTable.ref_count, 0)))
        .returning()
    },
  }) as const
