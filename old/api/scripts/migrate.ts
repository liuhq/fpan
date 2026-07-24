import { eq } from "drizzle-orm"
import { migrate } from "drizzle-orm/libsql/migrator"

import { ROOT } from "../constants.ts"
import { db } from "../db/client.ts"
import { folderTable } from "../db/schema.ts"
import { triggerStatements } from "../db/triggers.ts"

// Migrate and generate schemas
await migrate(db, { migrationsFolder: "./drizzle" })
console.log("✓ schema migrated")

// Seed 'ROOT'
const root = await db.select().from(folderTable).where(eq(folderTable.id, ROOT)).get()

if (!root) {
  await db.insert(folderTable).values({
    id: ROOT,
    parent_id: null,
    name: "/",
  })
  console.log(`✓ root(${ROOT}) folder seeded`)
} else {
  console.log(`• root(${ROOT}) folder already exists`)
}

// Register triggers
await db.$client.batch(triggerStatements, "write")
console.log(`✓ ${triggerStatements.length} triggers ensured`)

console.log("done.")
