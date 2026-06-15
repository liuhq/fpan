import { sql } from "drizzle-orm"
import { drizzle } from "drizzle-orm/libsql/sqlite3"

import * as schema from "./schema.ts"

/**
 * create a db client via drizzle, connect to `DB_FILE_NAME`
 * @returns Drizzle client
 */
export const createDbClient = async () => {
  const db = drizzle({
    connection: {
      url: Deno.env.get("DB_FILE_NAME")!,
    },
    schema,
  })

  await db.run(sql`PRAGMA foreign_keys = ON`)

  return db
}

export type DbClient = Awaited<ReturnType<typeof createDbClient>>
