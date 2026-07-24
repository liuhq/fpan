import { nanoid } from "nanoid"

import app from "./app.ts"
import type { entryTable } from "./db/schema.ts"

const file: typeof entryTable.$inferInsert = {
  id: nanoid(),
  name: "test.ts",
  created_at: Deno.
}

Deno.serve(app.fetch)
