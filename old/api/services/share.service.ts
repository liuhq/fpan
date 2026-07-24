import { and, eq, gt, isNull, lt, or, sql } from "drizzle-orm"

import type { DbClient } from "#db/client.ts"
import { shareTable } from "#db/schema.ts"

export type InsertShare = typeof shareTable.$inferInsert
export type SelectShare = typeof shareTable.$inferSelect

export const createShareService = (db: DbClient) =>
  ({
    create: async (share: InsertShare) => {
      return await db.insert(shareTable).values(share).returning()
    },
    recordDownload: async (token: SelectShare["token"]) => {
      const now = new Date()
      return await db
        .update(shareTable)
        .set({
          download_count: sql`${shareTable.download_count} + 1`,
        })
        .where(
          and(
            eq(shareTable.token, token),
            or(isNull(shareTable.expires_at), gt(shareTable.expires_at, now)),
            or(
              isNull(shareTable.download_limit),
              lt(shareTable.download_count, shareTable.download_limit),
            ),
            isNull(shareTable.revoked_at),
          ),
        )
        .returning()
    },
    revoke: async (token: SelectShare["token"]) => {
      return await db
        .update(shareTable)
        .set({
          revoked_at: new Date(),
        })
        .where(eq(shareTable.token, token))
        .returning()
    },
    findById: async (file_id: SelectShare["file_id"]) => {
      return await db.select().from(shareTable).where(eq(shareTable.file_id, file_id))
    },
    findByToken: async (token: SelectShare["token"]) => {
      return await db.select().from(shareTable).where(eq(shareTable.token, token))
    },
  }) as const
