import { mkdir, readFile, writeFile } from "node:fs/promises"

import openapiTS, { astToString, COMMENT_HEADER } from "openapi-typescript"
import ts from "typescript"

const source = new URL("../../api/openapi.yaml", import.meta.url)
const output = new URL("../app/openapi/schema.d.ts", import.meta.url)
const checkOnly = process.argv.includes("--check")

const ast = await openapiTS(source, {
  transform(schemaObject) {
    if (schemaObject.type === "string" && schemaObject.format === "binary") {
      return ts.factory.createTypeReferenceNode("Blob")
    }
  },
})
const generated = COMMENT_HEADER + astToString(ast)

if (checkOnly) {
  let current
  try {
    current = await readFile(output, "utf8")
  } catch (error) {
    if (error?.code !== "ENOENT") {
      throw error
    }
  }
  if (current !== generated) {
    console.error("OpenAPI types are stale. Run `pnpm api:generate`.")
    process.exitCode = 1
  }
} else {
  await mkdir(new URL("../app/openapi/", import.meta.url), { recursive: true })
  await writeFile(output, generated)
  console.log("Generated app/openapi/schema.d.ts")
}
