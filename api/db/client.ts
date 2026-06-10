import { sql } from "drizzle-orm"
import { drizzle } from "drizzle-orm/libsql/sqlite3"

import * as schema from "./schema.ts"

export const db = drizzle({
  connection: {
    url: Deno.env.get("DB_FILE_NAME")!,
  },
  schema,
})

await db.run(sql`PRAGMA foreign_keys = ON`)
